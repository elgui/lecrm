package sync

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ConnectionStatus tracks the lifecycle of a tenant's link to an
// external system.
type ConnectionStatus string

const (
	StatusPending      ConnectionStatus = "pending"       // OAuth flow started, not completed
	StatusActive       ConnectionStatus = "active"        // credentials valid, sync enabled
	StatusPaused       ConnectionStatus = "paused"        // user-initiated pause
	StatusError        ConnectionStatus = "error"         // last sync failed; retrying
	StatusRevoked      ConnectionStatus = "revoked"       // credentials invalidated by provider
	StatusDisconnected ConnectionStatus = "disconnected"  // user removed the connection
)

// Connection represents a tenant's active link to an external system.
// One workspace can have multiple connections (e.g., Gmail + Shopify).
// One workspace can have at most one connection per provider (enforced
// by UNIQUE constraint on (workspace, provider_id) in the DB).
type Connection struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	ProviderID  ProviderID
	Status      ConnectionStatus
	Settings    json.RawMessage // provider-specific config (e.g., which Gmail labels to import)
	SyncCursor  json.RawMessage // opaque cursor from last successful pull
	LastSyncAt  *time.Time
	LastError   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ConnectionStore abstracts persistence for sync connections.
// Implemented by sqlc-generated code in the workspace schema.
type ConnectionStore interface {
	CreateConnection(ctx context.Context, conn *Connection) error
	GetConnection(ctx context.Context, workspaceID uuid.UUID, providerID ProviderID) (*Connection, error)
	GetConnectionByID(ctx context.Context, id uuid.UUID) (*Connection, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status ConnectionStatus, lastError *string) error
	UpdateCursor(ctx context.Context, id uuid.UUID, cursor json.RawMessage, syncedAt time.Time) error
	ListConnections(ctx context.Context, workspaceID uuid.UUID) ([]*Connection, error)
}

// MappingStore abstracts persistence for external entity ID mappings.
// Implemented by sqlc-generated code in the workspace schema.
type MappingStore interface {
	// Upsert creates or updates a mapping between an external ID and a leCRM entity.
	UpsertMapping(ctx context.Context, m *EntityMapping) error

	// LookupByExternalID finds the leCRM entity mapped to an external ID.
	// Returns nil if no mapping exists.
	LookupByExternalID(ctx context.Context, providerID ProviderID, externalID string) (*EntityMapping, error)

	// LookupByEntityID finds external IDs mapped to a leCRM entity.
	LookupByEntityID(ctx context.Context, entityType string, entityID uuid.UUID) ([]*EntityMapping, error)
}

// EntityMapping links an external system entity to a leCRM entity.
type EntityMapping struct {
	ID           uuid.UUID
	ProviderID   ProviderID
	ExternalID   string
	EntityType   string // "contact", "company", "deal"
	EntityID     uuid.UUID
	LastSyncedAt time.Time
	Meta         json.RawMessage // provider-specific metadata about this mapping
	CreatedAt    time.Time
}
