// Command lecrm-mcp is the leCRM Model Context Protocol adapter: a
// standalone binary that exposes read-only CRM data to AI agents over a
// Streamable HTTP transport (ADR-011 — AI-native interface seam).
//
// It connects to Postgres as the constrained `lecrm_cube_reader` login
// role and assumes a per-workspace read-only role (workspace_<id>_ro)
// for the lifetime of each query, so it can never write CRM data. The
// rich mark3labs/mcp-go SDK is the intended dependency for v1; the v0
// skeleton speaks the MCP wire protocol directly to keep the binary
// hermetic and pinned to the repo's Go toolchain.
//
// Configuration (environment):
//
//	LECRM_MCP_DATABASE_URL         pgx connection string for lecrm_cube_reader (required)
//	LECRM_MCP_ADDR                 listen address (default ":8081")
//	LECRM_MCP_RATE_PER_SEC         per (workspace,token) token-bucket rate (default 20)
//	LECRM_MCP_RATE_BURST           per (workspace,token) bucket capacity (default 40)
//
// Write surface (ADR-012 §3 — optional; absent ⇒ read-only deployment):
//
//	LECRM_MCP_WRITE_DATABASE_URL   pgx connection string for a WRITE-capable
//	                               login role (mutates the workspace schema).
//	                               When set, the intent write tools
//	                               (advance_deal, log_interaction,
//	                               capture_lead) are enabled.
//	LECRM_MCP_CONFIRM_SECRET       HMAC secret for the destructive-op
//	                               confirmation handshake. REQUIRED when the
//	                               write URL is set (fail-closed otherwise).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/capability"
	"github.com/gbconsult/lecrm/apps/mcp/internal/mcpserver"
	"github.com/gbconsult/lecrm/apps/mcp/internal/ratelimit"
	"github.com/gbconsult/lecrm/apps/mcp/internal/store"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "0.1.0-skeleton"

func main() {
	// `lecrm-mcp healthcheck` probes the local /healthz endpoint and exits
	// 0/1. Used as the Compose healthcheck command since the distroless
	// runtime image has no shell or wget.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// healthcheck issues a GET against the local /healthz and returns a
// process exit code (0 = healthy).
func healthcheck() int {
	addr := envOr("LECRM_MCP_ADDR", ":8081")
	url := "http://127.0.0.1" + addr + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

func run(logger *slog.Logger) error {
	dbURL := os.Getenv("LECRM_MCP_DATABASE_URL")
	if dbURL == "" {
		return errors.New("LECRM_MCP_DATABASE_URL is required")
	}
	addr := envOr("LECRM_MCP_ADDR", ":8081")
	ratePerSec := envFloat("LECRM_MCP_RATE_PER_SEC", 20)
	burst := envFloat("LECRM_MCP_RATE_BURST", 40)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	logger.Info("database connected (read-only reader role)")

	// The MCP adapter is a thin projection over the shared capability
	// layer (ADR-012 §1). It links — does not re-implement — CRM reads.
	// The pool logs in as lecrm_cube_reader; per-read the capability layer
	// assumes the workspace_<id>_ro role (migration 0013), so the DB
	// enforces SELECT-only access.
	capSvc := capability.New(pool, logger)

	// Optional write surface (ADR-012 §3). When a write-capable DB URL is
	// configured, build a second capability Service on a pool whose login
	// role can mutate the workspace schema, plus the confirmation Confirmer
	// the destructive-op handshake needs. Absent ⇒ read-only deployment.
	var writer store.Writer
	if writeURL := os.Getenv("LECRM_MCP_WRITE_DATABASE_URL"); writeURL != "" {
		secret := os.Getenv("LECRM_MCP_CONFIRM_SECRET")
		confirmer, err := capability.NewConfirmer([]byte(secret))
		if err != nil {
			return fmt.Errorf("write surface enabled but LECRM_MCP_CONFIRM_SECRET invalid: %w", err)
		}
		writePool, err := pgxpool.New(ctx, writeURL)
		if err != nil {
			return fmt.Errorf("connect write db: %w", err)
		}
		defer writePool.Close()
		if err := writePool.Ping(ctx); err != nil {
			return fmt.Errorf("ping write db: %w", err)
		}
		writer = &store.CapabilityWriter{
			Svc:       capability.New(writePool, logger),
			Confirmer: confirmer,
		}
		logger.Info("write surface enabled (intent tools: advance_deal, log_interaction, capture_lead)")
	} else {
		logger.Info("read-only deployment (no write surface configured)")
	}

	srv := mcpserver.New(mcpserver.Config{
		Reader:  &store.CapabilityReader{Svc: capSvc},
		Writer:  writer,
		Limiter: ratelimit.New(ratePerSec, burst),
		Logger:  logger,
		Name:    "lecrm-mcp",
		Version: version,
	})

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		logger.Info("mcp listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown initiated")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
