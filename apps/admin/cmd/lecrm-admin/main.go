// Command lecrm-admin is the integrator-handoff CLI for leCRM v0
// (Story 8.1). It provisions, lists, inspects, and verifies tenants by
// composing the SECURITY DEFINER wrapper
// core.lecrm_provision_workspace_with_registry inside a single Postgres
// transaction.
//
// Léo's Tuesday-morning shape (per docs/integrator-handoff.md):
//
//	gb-tenant create --slug chauvet79 --admin-email leo@vernayo.com
//
// where `gb-tenant` is shell-aliased to
// `ssh dokku@54.37.157.49 run lecrm-admin /app/lecrm-admin tenant`.
//
// Subcommands:
//
//	tenant create  — provision a tenant (AC-F1..F5, AC-T1)
//	tenant verify  — run 14 invariants AC-I-01..AC-I-14
//	tenant list    — list all tenants in core.workspaces
//	tenant get     — show one tenant's metadata
//
// Environment variables:
//
//	DATABASE_URL or LECRM_PROVISIONER_DSN — connection string for the
//	  lecrm_provisioner role (required). DATABASE_URL is checked first
//	  for Dokku compatibility; the provisioner DSN wins if both are set.
//	LECRM_LOG_LEVEL — info | warn | error (default: info)
//
// AC-D5: the binary refuses to start if any LECRM_API_* env var is
// present — defense-in-depth against same-image binary co-location.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	cli "github.com/urfave/cli/v2"

	"github.com/gbconsult/lecrm/apps/admin/internal/safety"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

func main() {
	if err := safety.CheckAPIEnvLeak(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLogLevel(os.Getenv("LECRM_LOG_LEVEL")),
	}))
	slog.SetDefault(logger)

	app := &cli.App{
		Name:    "lecrm-admin",
		Usage:   "Integrator-handoff CLI for leCRM v0 (Story 8.1)",
		Version: "0.1.0",
		Commands: []*cli.Command{
			{
				Name:        "tenant",
				Usage:       "Tenant lifecycle subcommands",
				Subcommands: tenantSubcommands(logger),
			},
		},
		// urfave/cli v2 default error printer is fine; we surface
		// structured errors via os.Exit(1) below.
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func tenantSubcommands(logger *slog.Logger) []*cli.Command {
	return []*cli.Command{
		{
			Name:  "create",
			Usage: "Provision a tenant in one transaction",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "slug", Usage: "Tenant slug (lowercase, 3-32 chars)", Required: true},
				&cli.StringFlag{Name: "admin-email", Usage: "Tenant admin email", Required: true},
				&cli.StringFlag{Name: "owner-email", Usage: "Integrator / creator email (defaults to --admin-email)"},
				&cli.StringFlag{Name: "display-name", Usage: "Display name (reserved for v1)"},
				&cli.StringFlag{Name: "template", Usage: "Provisioning template", Value: templates.GBConsultDefaultName},
				&cli.BoolFlag{Name: "force-recreate", Usage: "Destroy existing tenant and recreate (atomic)"},
				&cli.BoolFlag{Name: "upsert", Usage: "Silent no-op if tenant already exists"},
			},
			Action: func(c *cli.Context) error {
				return runCreate(c, logger)
			},
		},
		{
			Name:  "verify",
			Usage: "Run 14 invariants against a tenant",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "slug", Usage: "Tenant slug", Required: true},
				&cli.BoolFlag{Name: "all-failures", Usage: "Report every failure instead of stopping at the first"},
			},
			Action: func(c *cli.Context) error {
				return runVerify(c, logger)
			},
		},
		{
			Name:   "list",
			Usage:  "List all tenants",
			Action: func(c *cli.Context) error { return runList(c, logger) },
		},
		{
			Name:  "get",
			Usage: "Show one tenant's metadata",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "slug", Usage: "Tenant slug", Required: true},
			},
			Action: func(c *cli.Context) error { return runGet(c, logger) },
		},
		{
			Name:  "tombstone",
			Usage: "Soft-delete a tenant (slug becomes permanently unavailable)",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "slug", Usage: "Tenant slug", Required: true},
			},
			Action: func(c *cli.Context) error { return runTombstone(c, logger) },
		},
	}
}

func runCreate(c *cli.Context, logger *slog.Logger) error {
	ctx := c.Context
	opts := tenant.CreateOptions{
		Slug:          c.String("slug"),
		AdminEmail:    c.String("admin-email"),
		OwnerEmail:    c.String("owner-email"),
		DisplayName:   c.String("display-name"),
		Template:      c.String("template"),
		ForceRecreate: c.Bool("force-recreate"),
		Upsert:        c.Bool("upsert"),
	}
	// Slug regex runs BEFORE opening the DB so an invalid slug never
	// reaches Postgres (Dev Notes hardening watch-item, AC-V1).
	if err := tenant.ValidateSlug(opts.Slug); err != nil {
		return err
	}
	conn, err := openConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	_, err = tenant.Create(ctx, conn, opts, os.Stdout)
	return err
}

func runVerify(c *cli.Context, logger *slog.Logger) error {
	ctx := c.Context
	opts := tenant.VerifyOptions{
		Slug:        c.String("slug"),
		AllFailures: c.Bool("all-failures"),
	}
	if err := tenant.ValidateSlug(opts.Slug); err != nil {
		return err
	}
	conn, err := openConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	result, err := tenant.Verify(ctx, conn, opts, os.Stdout)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return cli.Exit(fmt.Sprintf("%d/%d invariants failed", result.Failed, result.Total), 1)
	}
	return nil
}

func runList(c *cli.Context, logger *slog.Logger) error {
	ctx := c.Context
	conn, err := openConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	return tenant.List(ctx, conn, os.Stdout)
}

func runGet(c *cli.Context, logger *slog.Logger) error {
	ctx := c.Context
	slug := c.String("slug")
	if err := tenant.ValidateSlug(slug); err != nil {
		return err
	}
	conn, err := openConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	return tenant.Get(ctx, conn, slug, os.Stdout)
}

func runTombstone(c *cli.Context, logger *slog.Logger) error {
	ctx := c.Context
	slug := c.String("slug")
	if err := tenant.ValidateSlug(slug); err != nil {
		return err
	}
	conn, err := openConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	_, err = tenant.Tombstone(ctx, conn, tenant.TombstoneOptions{Slug: slug}, os.Stdout)
	return err
}

func openConn(ctx context.Context) (*pgx.Conn, error) {
	dsn := os.Getenv("LECRM_PROVISIONER_DSN")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return nil, tenant.New(tenant.ErrKindDBConnect,
			"LECRM_PROVISIONER_DSN (or DATABASE_URL) must be set")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, tenant.New(tenant.ErrKindDBConnect, "connect: %v", err)
	}
	return conn, nil
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
