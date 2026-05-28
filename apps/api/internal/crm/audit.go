package crm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
)

// emitAudit writes a single row into core.audit_log inside the caller's
// transaction. ADR-009 §7.2 fail-closed invariant: when this insert
// fails, the surrounding writeTx rolls back — the CRM mutation is
// rejected and no data is persisted.
//
// actor_type is derived from the bearer-token actor when one is
// present in context (set by workspace.MiddlewareWithBearer), and
// defaults to 'human_api' otherwise (session-cookie / browser path).
// Session user attribution (actor_user_id) will be plumbed through
// once the session middleware is extended to deposit a user identity
// into the request context (Sprint 9+). Leaving it NULL today is
// acceptable per the audit_log schema (actor_user_id is nullable).
func emitAudit(ctx context.Context, tx pgx.Tx, event string, workspaceID uuid.UUID, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("audit marshal %s: %w", event, err)
	}
	actorType := "human_api"
	if a, ok := auth.BearerActorFromContext(ctx); ok {
		actorType = a.ActorType
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO core.audit_log (event, workspace_id, actor_type, payload)
		 VALUES ($1, $2, $3, $4)`,
		event, workspaceID, actorType, body,
	); err != nil {
		return fmt.Errorf("audit insert %s: %w", event, err)
	}
	return nil
}
