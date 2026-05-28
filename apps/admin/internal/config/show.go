package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
)

type ShowOptions struct {
	Slug    string
	Version int // 0 = latest
}

func Show(ctx context.Context, conn *pgx.Conn, opts ShowOptions, stdout io.Writer) (*MethodologyConfig, error) {
	ref, err := ResolveSlug(ctx, conn, opts.Slug)
	if err != nil {
		return nil, err
	}

	cfg, err := loadConfig(ctx, conn, ref, opts.Version)
	if err != nil {
		return nil, err
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "Tenant %q — methodology config v%d\n\n", opts.Slug, cfg.VersionSeq)
	_, _ = stdout.Write(out)
	_, _ = fmt.Fprintln(stdout)
	return cfg, nil
}

func loadConfig(ctx context.Context, conn *pgx.Conn, ref WorkspaceRef, version int) (*MethodologyConfig, error) {
	var q string
	var args []any

	if version > 0 {
		q = fmt.Sprintf(
			`SELECT data FROM %s.objects WHERE object_type = $1 AND (data->>'version_seq')::int = $2`,
			safeIdent(ref.RoleName))
		args = []any{ObjectType, version}
	} else {
		q = fmt.Sprintf(
			`SELECT data FROM %s.objects WHERE object_type = $1 ORDER BY (data->>'version_seq')::int DESC LIMIT 1`,
			safeIdent(ref.RoleName))
		args = []any{ObjectType}
	}

	var raw []byte
	if err := conn.QueryRow(ctx, q, args...).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("tenant %q has no methodology config", ref.Slug)
		}
		return nil, fmt.Errorf("query config: %w", err)
	}

	var cfg MethodologyConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
