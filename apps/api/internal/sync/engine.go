package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Engine orchestrates the sync lifecycle: pull → match → map → apply.
// It delegates external-system communication to Provider and persistence
// to ConnectionStore/MappingStore. All workspace-scoped operations run
// inside a RunWorkspaceJob envelope (advisory lock + search_path guard).
//
// This is a stub — the Run method documents the orchestration contract
// but does not execute sync logic. Implementation lands when the OAuth
// surface (tasket bf09) and secrets baseline (tasket 1023) are ready.
type Engine struct {
	registry *Registry
	conns    ConnectionStore
	mappings MappingStore
	logger   *slog.Logger
}

// NewEngine creates a sync engine wired to the given stores.
func NewEngine(
	registry *Registry,
	conns ConnectionStore,
	mappings MappingStore,
	logger *slog.Logger,
) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		registry: registry,
		conns:    conns,
		mappings: mappings,
		logger:   logger,
	}
}

// RunSync executes a single sync cycle for one connection.
//
// Orchestration contract (implemented when OAuth + secrets land):
//
//  1. Load connection from ConnectionStore.
//  2. Resolve provider from Registry.
//  3. Validate credentials (Provider.ValidateCredentials).
//     On failure → update connection status to Error/Revoked, return.
//  4. Pull changes (Provider.Pull).
//     Respects rate limits via context deadline.
//  5. For each InboundRecord:
//     a. Check MappingStore for existing external_id → entity_id mapping.
//     b. If no mapping: call Provider.Match for fuzzy resolution.
//     c. If match found with sufficient confidence: create mapping.
//     d. If no match: create new entity + mapping.
//     e. If match below confidence threshold: log and skip (v0 policy).
//     f. Apply record fields to entity via entity-type writer.
//     g. Upsert mapping with updated last_synced_at.
//  6. Update connection cursor + last_sync_at.
//  7. On error at any step: update connection status, log, return error.
//
// All steps run inside a workspace-scoped advisory lock (via the jobs
// package) to prevent concurrent syncs for the same tenant.
func (e *Engine) RunSync(ctx context.Context, connectionID uuid.UUID) error {
	conn, err := e.conns.GetConnectionByID(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("sync: load connection %s: %w", connectionID, err)
	}

	provider, err := e.registry.Get(conn.ProviderID)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	e.logger.InfoContext(ctx, "sync: starting",
		slog.String("connection_id", connectionID.String()),
		slog.String("provider", string(conn.ProviderID)),
		slog.String("workspace_id", conn.WorkspaceID.String()),
	)

	if err := provider.ValidateCredentials(ctx, conn); err != nil {
		_ = e.conns.UpdateStatus(ctx, conn.ID, StatusError, strPtr(err.Error()))
		return fmt.Errorf("sync: credentials invalid for %s: %w", conn.ProviderID, err)
	}

	result, err := provider.Pull(ctx, conn)
	if err != nil {
		_ = e.conns.UpdateStatus(ctx, conn.ID, StatusError, strPtr(err.Error()))
		return fmt.Errorf("sync: pull failed for %s: %w", conn.ProviderID, err)
	}

	for _, rec := range result.Records {
		if err := e.processRecord(ctx, provider, conn, rec); err != nil {
			e.logger.WarnContext(ctx, "sync: record processing failed",
				slog.String("external_id", rec.ExternalID),
				slog.String("entity_type", rec.EntityType),
				slog.String("error", err.Error()),
			)
			// Continue processing remaining records — one failure shouldn't
			// halt the entire sync batch.
		}
	}

	now := time.Now()
	if err := e.conns.UpdateCursor(ctx, conn.ID, result.Cursor, now); err != nil {
		return fmt.Errorf("sync: update cursor for %s: %w", connectionID, err)
	}

	e.logger.InfoContext(ctx, "sync: completed",
		slog.String("connection_id", connectionID.String()),
		slog.Int("records_processed", len(result.Records)),
	)
	return nil
}

const matchConfidenceThreshold = 0.8

func (e *Engine) processRecord(ctx context.Context, provider Provider, conn *Connection, rec InboundRecord) error {
	existing, err := e.mappings.LookupByExternalID(ctx, conn.ProviderID, rec.ExternalID)
	if err != nil {
		return fmt.Errorf("lookup mapping: %w", err)
	}

	if existing != nil {
		// TODO: update existing entity with rec.Fields via entity writer
		return e.mappings.UpsertMapping(ctx, &EntityMapping{
			ID:           existing.ID,
			ProviderID:   conn.ProviderID,
			ExternalID:   rec.ExternalID,
			EntityType:   rec.EntityType,
			EntityID:     existing.EntityID,
			LastSyncedAt: time.Now(),
			Meta:         rec.Meta,
		})
	}

	match, err := provider.Match(ctx, conn, rec)
	if err != nil {
		return fmt.Errorf("match: %w", err)
	}

	if match != nil && match.Confidence >= matchConfidenceThreshold {
		// TODO: update matched entity with rec.Fields via entity writer
		return e.mappings.UpsertMapping(ctx, &EntityMapping{
			ProviderID:   conn.ProviderID,
			ExternalID:   rec.ExternalID,
			EntityType:   match.EntityType,
			EntityID:     match.EntityID,
			LastSyncedAt: time.Now(),
			Meta:         rec.Meta,
		})
	}

	if match != nil {
		e.logger.InfoContext(ctx, "sync: low-confidence match, skipping",
			slog.String("external_id", rec.ExternalID),
			slog.Float64("confidence", match.Confidence),
		)
		return nil
	}

	// TODO: create new entity via entity writer, then create mapping
	e.logger.InfoContext(ctx, "sync: no match found, entity creation deferred",
		slog.String("external_id", rec.ExternalID),
		slog.String("entity_type", rec.EntityType),
	)
	return nil
}

func strPtr(s string) *string { return &s }
