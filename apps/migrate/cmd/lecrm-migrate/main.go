// Command lecrm-migrate is the leCRM DDL runner and workspace
// provisioner. It runs as the lecrm_provisioner role (Tier-0 secret per
// ADR-007) and is invoked as a Compose pre-deploy job before lecrm-api
// starts (ADR-009 §8.2).
//
// Subcommands:
//
//	apply                  — apply pending SQL migrations from LECRM_MIGRATIONS_DIR
//	provision-workspace    — provision a workspace role+schema (idempotent)
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

	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/migrate/internal/migrator"
	"github.com/gbconsult/lecrm/apps/migrate/internal/provision"
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
		return fmt.Errorf("usage: lecrm-migrate <apply|provision-workspace> [flags]")
	}

	switch args[0] {
	case "apply":
		return cmdApply(logger, args[1:])
	case "provision-workspace":
		return cmdProvisionWorkspace(logger, args[1:])
	default:
		return fmt.Errorf("unknown subcommand %q; want apply or provision-workspace", args[0])
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
	defer conn.Close(context.Background())

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
	defer conn.Close(ctx)

	result, err := provision.Run(ctx, conn, *slug, logger)
	if err != nil {
		return err
	}

	logger.Info("done",
		"workspace_id", result.WorkspaceID,
		"slug", result.Slug,
		"role", result.RoleName,
		"is_new", result.IsNew)
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
