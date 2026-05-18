// Package migrator applies versioned SQL migration files to Postgres.
// It tracks applied files in core.schema_migrations and continues past
// individual failures (on_error = CONTINUE per ADR-009 §2.4).
package migrator

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Apply reads *.sql files from migrationsDir in alphanumeric order and
// applies those not yet in core.schema_migrations. Returns a non-nil
// error only if all files that needed running failed; partial failure is
// logged per file and execution continues (ADR-009 §2.4 on_error=CONTINUE).
func Apply(ctx context.Context, conn *pgx.Conn, migrationsDir string, logger *slog.Logger) error {
	if err := ensureTrackingTable(ctx, conn); err != nil {
		return fmt.Errorf("migrator: ensure tracking table: %w", err)
	}

	files, err := sqlFiles(migrationsDir)
	if err != nil {
		return fmt.Errorf("migrator: list files: %w", err)
	}

	applied, err := appliedSet(ctx, conn)
	if err != nil {
		return fmt.Errorf("migrator: list applied: %w", err)
	}

	var errs []string
	ran := 0
	for _, f := range files {
		name := filepath.Base(f)
		if applied[name] {
			logger.InfoContext(ctx, "migration already applied", "file", name)
			continue
		}

		logger.InfoContext(ctx, "applying migration", "file", name)
		if err := applyFile(ctx, conn, f, name); err != nil {
			logger.ErrorContext(ctx, "migration failed", "file", name, "err", err)
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		ran++
		logger.InfoContext(ctx, "migration applied", "file", name)
	}

	logger.InfoContext(ctx, "migrations complete", "applied", ran, "failures", len(errs))
	if len(errs) > 0 {
		return fmt.Errorf("migrator: %d file(s) failed: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func ensureTrackingTable(ctx context.Context, conn *pgx.Conn) error {
	if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS core"); err != nil {
		return err
	}
	_, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS core.schema_migrations (
			name       TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

func sqlFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(d.Name(), ".sql") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func appliedSet(ctx context.Context, conn *pgx.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT name FROM core.schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func applyFile(ctx context.Context, conn *pgx.Conn, path, name string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	if _, err := conn.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	_, err = conn.Exec(ctx,
		"INSERT INTO core.schema_migrations (name) VALUES ($1) ON CONFLICT DO NOTHING", name)
	return err
}
