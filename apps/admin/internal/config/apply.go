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

type ApplyOptions struct {
	Slug     string
	Template string
}

type ApplyResult struct {
	Slug       string
	VersionSeq int
}

func Apply(ctx context.Context, conn *pgx.Conn, opts ApplyOptions, stdout io.Writer) (*ApplyResult, error) {
	ref, err := ResolveSlug(ctx, conn, opts.Slug)
	if err != nil {
		return nil, err
	}

	tpl, ok := Templates[opts.Template]
	if !ok {
		known := make([]string, 0, len(Templates))
		for k := range Templates {
			known = append(known, k)
		}
		return nil, fmt.Errorf("unknown template %q; known: %v", opts.Template, known)
	}

	nextSeq, err := nextVersionSeq(ctx, conn, ref)
	if err != nil {
		return nil, err
	}

	cfg := tpl
	cfg.VersionSeq = nextSeq

	raw, err := json.Marshal(&cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("mint UUIDv7: %w", err)
	}

	q := fmt.Sprintf(
		`INSERT INTO %s.objects (id, object_type, data, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4)`,
		safeIdent(ref.RoleName))
	now := time.Now().UTC()
	if _, err := conn.Exec(ctx, q, id, ObjectType, raw, now); err != nil {
		return nil, fmt.Errorf("insert config: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Applied template %q to tenant %q (v%d)\n", opts.Template, opts.Slug, nextSeq)
	_, _ = fmt.Fprintf(stdout, "OBJECT_ID=%s\n", id)

	if _, err := provisionCustomProperties(ctx, conn, ref, &cfg, stdout); err != nil {
		return nil, fmt.Errorf("provision custom properties: %w", err)
	}

	if err := emitAudit(ctx, conn, ref, "config.template.applied", map[string]any{
		"template":    opts.Template,
		"version_seq": nextSeq,
		"object_id":   id.String(),
	}); err != nil {
		return nil, err
	}

	return &ApplyResult{Slug: opts.Slug, VersionSeq: nextSeq}, nil
}
