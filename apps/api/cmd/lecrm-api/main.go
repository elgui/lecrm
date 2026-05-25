// Command lecrm-api is the leCRM v0 HTTP server: REST endpoints under
// /v1/* and (post-Sprint-2) the embedded React SPA under /*.
//
// At Day-2 this binary serves only the /auth/* OIDC flow and a healthz
// probe; REST handlers land in Sprint 7.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/config"
	"github.com/gbconsult/lecrm/apps/api/internal/db"
	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	httpserver "github.com/gbconsult/lecrm/apps/api/internal/http"
	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()
	logger.Info("database connected")

	// The redirect URI is computed per-request in the handler from the
	// inbound workspace subdomain — Authentik rejects mismatches. We
	// pass a sentinel here only because rp.NewRelyingPartyOIDC requires
	// a non-empty value; AuthURL/CodeExchange always override it.
	placeholderRedirectURI := fmt.Sprintf("https://placeholder.%s%s", cfg.CookieDomainTLD, cfg.OIDC.CallbackPath)
	provider, err := auth.NewProvider(ctx, cfg.OIDC.Issuer, cfg.OIDC.ClientID, cfg.OIDC.ClientSecret, placeholderRedirectURI, cfg.OIDC.Scopes, cfg.SessionSecret)
	if err != nil {
		return fmt.Errorf("oidc provider: %w", err)
	}
	logger.Info("oidc provider ready", "issuer", cfg.OIDC.Issuer)

	authH := &auth.Handler{
		Provider:      provider,
		Store:         auth.NewStore(pool),
		SessionSecret: cfg.SessionSecret,
		DomainTLD:     cfg.CookieDomainTLD,
		CookieSecure:  cfg.CookieSecure,
		CallbackPath:  cfg.OIDC.CallbackPath,
		Logger:        logger,
	}

	wsResolver := &workspace.PoolResolver{Pool: pool}
	testList := &workspace.TestListHandler{Pool: pool, Logger: logger}
	metadataH := &metadata.Handler{Pool: pool, Logger: logger}
	crmH := &crm.Handler{Pool: pool, Logger: logger}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewRouter(httpserver.RouterDeps{
			Logger:          logger,
			AuthHandler:     authH,
			Resolver:        wsResolver,
			TestList:        testList,
			Metadata:        metadataH,
			CRM:             crmH,
			CookieDomainTLD: cfg.CookieDomainTLD,
		}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown initiated")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}
