// Package sync defines the external-system-sync abstraction for leCRM.
//
// Every connector (Gmail, Shopify, etc.) implements Provider. The sync
// engine orchestrates: pull external changes → match to leCRM entities
// → create/update mappings → apply writes. Providers are decoupled from
// the CRM data layer; they produce normalized InboundRecords that the
// engine routes to entity-specific writers.
//
// See ADR-011 for the full design rationale and Shopify paper exercise.
package sync

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ProviderID uniquely identifies a connector type (e.g., "gmail", "shopify").
type ProviderID string

const (
	ProviderGmail   ProviderID = "gmail"
	ProviderShopify ProviderID = "shopify"
)

// SyncDirection indicates which way data flows through the connector.
type SyncDirection int

const (
	Inbound SyncDirection = iota // external → leCRM (read-only import)
	Outbound                     // leCRM → external (write-back)
	Bidir                        // both directions
)

func (d SyncDirection) String() string {
	switch d {
	case Inbound:
		return "inbound"
	case Outbound:
		return "outbound"
	case Bidir:
		return "bidirectional"
	default:
		return "unknown"
	}
}

// ChangeAction describes what happened to the external entity.
type ChangeAction int

const (
	Created ChangeAction = iota
	Updated
	Deleted
)

// InboundRecord is a single entity fetched from the external system,
// normalized into a provider-agnostic shape. The sync engine uses
// MatchHint to locate the corresponding leCRM entity, then routes
// Fields to the appropriate entity writer.
type InboundRecord struct {
	ExternalID string
	EntityType string // "contact", "company", "deal", "thread", "order", etc.
	Action     ChangeAction
	Fields     map[string]any  // normalized field data (provider maps its schema here)
	MatchHint  MatchHint       // how to find the leCRM counterpart
	Meta       json.RawMessage // provider-specific metadata (stored but not interpreted by engine)
	OccurredAt time.Time
}

// MatchHint tells the sync engine how to locate a matching leCRM entity.
// Strategy selects the matching algorithm; Value is the input to that
// algorithm (e.g., an email address for "email" strategy).
type MatchHint struct {
	Strategy string // "email", "domain", "external_id", "phone"
	Value    string
}

// PullResult contains changes fetched from an external system and an
// opaque cursor for the next incremental pull.
type PullResult struct {
	Records []InboundRecord
	Cursor  json.RawMessage // opaque; stored by engine, passed back on next run
}

// EntityMatch is the result of resolving an external entity to a leCRM
// record. Nil means no match was found (the engine should create a new entity).
type EntityMatch struct {
	EntityType string
	EntityID   uuid.UUID
	Confidence float64 // 0.0–1.0; below threshold → skip or surface to user
}

// Provider is the interface every external-system connector must implement.
// It knows how to talk to one external API and normalize responses into
// InboundRecords. It does NOT write to the leCRM database — the sync
// engine handles that.
type Provider interface {
	// ID returns the unique provider identifier.
	ID() ProviderID

	// Direction returns the sync direction supported by this provider.
	// v0 providers return Inbound; bidirectional support is a v1+ concern.
	Direction() SyncDirection

	// Pull fetches changes from the external system since the cursor
	// position stored in conn.SyncCursor. Returns normalized records
	// and an updated cursor. conn.SyncCursor is nil on the first pull.
	Pull(ctx context.Context, conn *Connection) (*PullResult, error)

	// Match attempts to find a leCRM entity corresponding to an inbound
	// record, using the record's MatchHint. Returns nil if no match is
	// found (the engine should create a new entity).
	Match(ctx context.Context, conn *Connection, rec InboundRecord) (*EntityMatch, error)

	// ValidateCredentials checks that the connection's stored credentials
	// are still usable (token not expired, scopes sufficient, etc.).
	ValidateCredentials(ctx context.Context, conn *Connection) error
}
