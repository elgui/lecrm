// Command lecrm-migrate is the leCRM DDL runner and workspace
// provisioner. It runs as the lecrm_provisioner role (Tier-0 secret per
// ADR-007) and is invoked as a Compose pre-deploy job before lecrm-api
// starts (ADR-009 §8.2).
//
// Subcommands:
//
//	apply                  — apply pending SQL migrations from LECRM_MIGRATIONS_DIR
//	provision-workspace    — provision a workspace role+schema (idempotent)
//	river-setup            — create River tables in workspaces' river_<hex>
//	                         schemas + grant lecrm_api (idempotent backfill)
//
// Environment variables:
//
//	LECRM_PROVISIONER_DSN  — Postgres DSN for the lecrm_provisioner role (required)
//	LECRM_MIGRATIONS_DIR   — path to directory containing *.sql migration files
//	                         (default: packages/db/migrations)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/migrate/internal/migrator"
	"github.com/gbconsult/lecrm/apps/migrate/internal/provision"
	"github.com/gbconsult/lecrm/apps/migrate/internal/riversetup"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: lecrm-migrate <apply|provision-workspace|river-setup> [flags]")
	}

	switch args[0] {
	case "apply":
		return cmdApply(logger, args[1:])
	case "provision-workspace":
		return cmdProvisionWorkspace(logger, args[1:])
	case "river-setup":
		return cmdRiverSetup(logger, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q; want apply, provision-workspace or river-setup", args[0])
	}
}

func cmdApply(logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	migrationsDir := fs.String("migrations-dir", envOr("LECRM_MIGRATIONS_DIR", "packages/db/migrations"),
		"directory containing *.sql migration files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	conn, err := openProvisionerConn(context.Background())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()

	return migrator.Apply(context.Background(), conn, *migrationsDir, logger)
}

func cmdProvisionWorkspace(logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("provision-workspace", flag.ContinueOnError)
	slug := fs.String("slug", "", "workspace slug to provision (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" {
		return fmt.Errorf("provision-workspace: --slug is required")
	}

	ctx := context.Background()
	conn, err := openProvisionerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	result, err := provision.Run(ctx, conn, *slug, logger)
	if err != nil {
		return err
	}

	// Create River's tables in the workspace's river_<hex> schema and grant
	// lecrm_api so the API's per-workspace river client can run. Idempotent, so
	// it is safe on re-provision of an existing workspace.
	if err := riversetup.SetupWorkspace(ctx, os.Getenv("LECRM_PROVISIONER_DSN"), result.WorkspaceID, result.RoleName, logger); err != nil {
		return fmt.Errorf("provision-workspace: river setup: %w", err)
	}

	logger.Info("done",
		"workspace_id", result.WorkspaceID,
		"slug", result.Slug,
		"role", result.RoleName,
		"is_new", result.IsNew)
	return nil
}

// cmdRiverSetup backfills River tables (+ lecrm_api grants) into the river_<hex>
// schema of one workspace (--slug) or every workspace in core.workspaces
// (--all). Idempotent; intended for existing workspaces provisioned before the
// river runtime landed.
func cmdRiverSetup(logger *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("river-setup", flag.ContinueOnError)
	slug := fs.String("slug", "", "workspace slug to set up (mutually exclusive with --all)")
	all := fs.Bool("all", false, "set up every workspace in core.workspaces")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*slug == "") == (!*all) {
		return fmt.Errorf("river-setup: pass exactly one of --slug or --all")
	}

	ctx := context.Background()
	dsn := os.Getenv("LECRM_PROVISIONER_DSN")
	if dsn == "" {
		return fmt.Errorf("LECRM_PROVISIONER_DSN is required")
	}
	conn, err := openProvisionerConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()

	query := "SELECT id, role_name FROM core.workspaces"
	queryArgs := []any{}
	if *slug != "" {
		query += " WHERE slug = $1"
		queryArgs = append(queryArgs, *slug)
	}
	rows, err := conn.Query(ctx, query, queryArgs...)
	if err != nil {
		return fmt.Errorf("river-setup: list workspaces: %w", err)
	}
	type target struct {
		id   uuid.UUID
		role string
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.role); err != nil {
			rows.Close()
			return fmt.Errorf("river-setup: scan workspace: %w", err)
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("river-setup: iterate workspaces: %w", err)
	}
	if len(targets) == 0 {
		return fmt.Errorf("river-setup: no matching workspace")
	}

	for _, t := range targets {
		if err := riversetup.SetupWorkspace(ctx, dsn, t.id, t.role, logger); err != nil {
			return fmt.Errorf("river-setup: workspace %s (%s): %w", t.id, t.role, err)
		}
	}
	logger.Info("river-setup done", "workspaces", len(targets))
	return nil
}

func openProvisionerConn(ctx context.Context) (*pgx.Conn, error) {
	dsn := os.Getenv("LECRM_PROVISIONER_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("LECRM_PROVISIONER_DSN is required")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect as provisioner: %w", err)
	}
	return conn, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
