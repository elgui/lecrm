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

	"github.com/gbconsult/lecrm/apps/api/internal/admin"
	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/config"
	"github.com/gbconsult/lecrm/apps/api/internal/crm"
	"github.com/gbconsult/lecrm/apps/api/internal/db"
	"github.com/gbconsult/lecrm/apps/api/internal/email"
	"github.com/gbconsult/lecrm/apps/api/internal/email/brevo"
	httpserver "github.com/gbconsult/lecrm/apps/api/internal/http"
	"github.com/gbconsult/lecrm/apps/api/internal/members"
	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
	"github.com/gbconsult/lecrm/apps/api/internal/rbac"
	"github.com/gbconsult/lecrm/apps/api/internal/reports"
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

	// Workspace service tokens (ADR-009 §4.1). The store wires both
	// the Bearer-token verifier (CandidateLoader) and the
	// /v1/workspace/tokens handler.
	tokenStore := &auth.PgServiceTokenStore{Pool: pool}
	tokensH := &auth.ServiceTokenHandler{Store: tokenStore, Logger: logger}
	bearerAuth := &auth.HTTPBearerAuthenticator{Loader: tokenStore}

	// Email handler. Brevo credentials and the inbound webhook HMAC
	// secret are read from env (a per-workspace secrets resolver lands
	// post-SOPS-rollout). When BREVO_API_KEY is empty we still mount the
	// handler — sends return ErrEmptyAPIKey explicitly.
	brevoClient := brevo.New(os.Getenv("BREVO_API_KEY"), os.Getenv("BREVO_BASE_URL"), nil)
	emailSvc := &email.Service{
		Provider:    brevoClient,
		Suppression: &email.PgSuppressionStore{Pool: pool},
		Audit:       &email.PgAuditWriter{Pool: pool},
		Logger:      logger,
	}
	emailH := &email.Handler{
		Service:       emailSvc,
		Logger:        logger,
		WebhookSource: email.StaticWebhookSecret([]byte(os.Getenv("BREVO_WEBHOOK_SECRET"))),
	}

	// Phase 3 integrator-handoff: /admin/audit. Token empty → handler
	// 503s. v1+ rotates to OIDC admin claims.
	adminH := &admin.AuditHandler{
		Pool:   pool,
		Token:  os.Getenv("LECRM_ADMIN_TOKEN"),
		Logger: logger,
	}

	// Cube.dev embed-token handler (ADR-009 §9). Wired only when
	// LECRM_CUBE_JWT_SECRET is set; the handler itself 503s if invoked
	// without a secret so a partial deploy fails loudly.
	reportsH := &reports.Handler{
		JWTSecret: cfg.CubeJWTSecret,
		TTL:       reports.DefaultTTL,
		DecodeSession: func(r *http.Request, slug string) (auth.Session, bool) {
			s, _, ok := auth.SessionFromRequestV2(r, slug, cfg.SessionSecret)
			return s, ok
		},
		Audit:  &reports.PgAuditWriter{Pool: pool},
		Logger: logger,
	}

	// Multi-user RBAC (ADR-009 §2, Sprint 8). A single PgMemberStore
	// backs both the authorization role lookup and member management.
	decodeSession := func(r *http.Request, slug string) (auth.Session, bool) {
		s, _, ok := auth.SessionFromRequestV2(r, slug, cfg.SessionSecret)
		return s, ok
	}
	memberStore := &members.PgMemberStore{Pool: pool}
	rbacResolver := &rbac.Resolver{
		Store:         memberStore,
		DecodeSession: decodeSession,
		Logger:        logger,
	}
	membersH := &members.Handler{
		Store:         memberStore,
		DecodeSession: decodeSession,
		Logger:        logger,
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpserver.NewRouter(httpserver.RouterDeps{
			Logger:          logger,
			AuthHandler:     authH,
			ServiceTokens:   tokensH,
			BearerAuth:      bearerAuth,
			Resolver:        wsResolver,
			TestList:        testList,
			Metadata:        metadataH,
			CRM:             crmH,
			Email:           emailH,
			Admin:           adminH,
			Reports:         reportsH,
			RBAC:            rbacResolver,
			Members:         membersH,
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
