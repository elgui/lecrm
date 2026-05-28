package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReplayOptions struct {
	SrcSlug string
	DstSlug string
}

type ReplayResult struct {
	SrcSlug    string
	DstSlug    string
	VersionSeq int
}

func Replay(ctx context.Context, conn *pgx.Conn, opts ReplayOptions, stdout io.Writer) (*ReplayResult, error) {
	src, err := ResolveSlug(ctx, conn, opts.SrcSlug)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	dst, err := ResolveSlug(ctx, conn, opts.DstSlug)
	if err != nil {
		return nil, fmt.Errorf("destination: %w", err)
	}

	srcCfg, err := loadConfig(ctx, conn, src, 0)
	if err != nil {
		return nil, fmt.Errorf("source %s: %w", opts.SrcSlug, err)
	}

	nextSeq, err := nextVersionSeq(ctx, conn, dst)
	if err != nil {
		return nil, err
	}

	replayed := *srcCfg
	replayed.VersionSeq = nextSeq

	raw, err := json.Marshal(&replayed)
	if err != nil {
		return nil, fmt.Errorf("marshal replayed config: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("mint UUIDv7: %w", err)
	}

	q := fmt.Sprintf(
		`INSERT INTO %s.objects (id, object_type, data, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4)`,
		safeIdent(dst.RoleName))
	now := time.Now().UTC()
	if _, err := conn.Exec(ctx, q, id, ObjectType, raw, now); err != nil {
		return nil, fmt.Errorf("insert replayed config: %w", err)
	}

	_, _ = fmt.Fprintf(stdout,
		"Replayed methodology config from %q (v%d) → %q (v%d)\n",
		opts.SrcSlug, srcCfg.VersionSeq, opts.DstSlug, nextSeq)
	_, _ = fmt.Fprintf(stdout, "OBJECT_ID=%s\n", id)

	if _, err := provisionCustomProperties(ctx, conn, dst, srcCfg, stdout); err != nil {
		return nil, fmt.Errorf("provision custom properties on %q: %w", opts.DstSlug, err)
	}

	if err := emitAudit(ctx, conn, dst, "config.template.replayed", map[string]any{
		"src_slug":        opts.SrcSlug,
		"src_version_seq": srcCfg.VersionSeq,
		"version_seq":     nextSeq,
		"object_id":       id.String(),
	}); err != nil {
		return nil, err
	}

	return &ReplayResult{
		SrcSlug:    opts.SrcSlug,
		DstSlug:    opts.DstSlug,
		VersionSeq: nextSeq,
	}, nil
}

func nextVersionSeq(ctx context.Context, conn *pgx.Conn, ref WorkspaceRef) (int, error) {
	q := fmt.Sprintf(
		`SELECT COALESCE(MAX((data->>'version_seq')::int), 0)
		   FROM %s.objects WHERE object_type = $1`,
		safeIdent(ref.RoleName))

	var maxSeq int
	if err := conn.QueryRow(ctx, q, ObjectType).Scan(&maxSeq); err != nil {
		return 0, fmt.Errorf("query max version_seq: %w", err)
	}
	return maxSeq + 1, nil
}
