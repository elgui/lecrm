package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

// OperatorEmailEnv is read by emitAudit to attribute config mutations
// to a human integrator (Léo for the Vernayo handoff). When unset the
// audit row carries "unknown" so the row is never lost.
const OperatorEmailEnv = "LECRM_OPERATOR_EMAIL"

// emitAudit writes a core.audit_log row for a Phase 2 config mutation.
// actor_type is always human_api (Phase 2 ops run from the integrator
// CLI, which is the only caller). Failure rolls the calling tx — the
// fail-closed invariant from ADR-009 §7.2 forbids silent audit loss.
func emitAudit(ctx context.Context, conn *pgx.Conn, ref WorkspaceRef, event string, extra map[string]any) error {
	payload := map[string]any{
		"slug":           ref.Slug,
		"role_name":      ref.RoleName,
		"operator_email": operatorEmail(),
	}
	for k, v := range extra {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal audit payload: %w", err)
	}
	_, err = conn.Exec(ctx,
		`INSERT INTO core.audit_log (event, workspace_id, actor_type, payload)
		 VALUES ($1, $2, 'human_api', $3)`,
		event, ref.ID, raw)
	if err != nil {
		return fmt.Errorf("insert audit row (%s): %w", event, err)
	}
	return nil
}

func operatorEmail() string {
	if v := os.Getenv(OperatorEmailEnv); v != "" {
		return v
	}
	return "unknown"
}
