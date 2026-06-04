// Package crm holds the thin HTTP handlers for the core CRM objects
// (contacts, companies, deals, notes, tasks, activities) and their
// mapping to the shared capability layer.
package crm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Actor types — must match the CHECK constraint in migration 0015.
const (
	actorTypeHumanAPI        = "human_api"
	actorTypeMCPAgent        = "mcp_agent"
	actorTypeInternalService = "internal_service"
	actorTypeSystem          = "system"
	actorTypeConnector       = "connector"
)

// Entity types — must match the CHECK constraint in migration 0015.
const (
	entityTypeContact = "contact"
	entityTypeCompany = "company"
	entityTypeDeal    = "deal"
)

// emitActivity writes a single row into the workspace's activities
// table inside the caller's transaction. The search_path is set by
// writeTx/readTx, so `activities` resolves to the workspace schema.
//
// Fail-closed: when this insert fails, the surrounding writeTx rolls
// back — the originating CRM mutation is rejected and no data is
// persisted. Same contract as emitAudit (ADR-009 §7.2).
//
// For the REST surface, actorType is fixed to "human_api" and
// sourceSystem is empty. Connector workers (Sprint 8+) supply
// actorType="connector" with sourceSystem populated (ADR-011 §3c).
func emitActivity(
	ctx context.Context,
	tx pgx.Tx,
	entityType string,
	entityID uuid.UUID,
	eventType string,
	actorType string,
	sourceSystem string,
	payload any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("activity marshal %s: %w", eventType, err)
	}
	var srcArg any
	if sourceSystem != "" {
		srcArg = sourceSystem
	}
	var actorArg any
	if actorType != "" {
		actorArg = actorType
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO activities (entity_type, entity_id, event_type, actor_type, source_system, payload)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		// string(body), not body: under pgx's simple query protocol a []byte
		// is sent as a bytea literal and rejected by the jsonb column (22P02).
		entityType, entityID, eventType, actorArg, srcArg, string(body),
	); err != nil {
		return fmt.Errorf("activity insert %s: %w", eventType, err)
	}
	return nil
}

// emitRESTActivity is a thin convenience that fixes actor_type to
// "human_api" and source_system to empty — the right default for the
// REST handlers in this package.
func emitRESTActivity(ctx context.Context, tx pgx.Tx, entityType string, entityID uuid.UUID, eventType string, payload any) error {
	return emitActivity(ctx, tx, entityType, entityID, eventType, actorTypeHumanAPI, "", payload)
}
