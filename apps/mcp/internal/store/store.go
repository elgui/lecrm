// Package store is the MCP adapter's thin seam onto the shared capability
// layer (apps/api/capability). It no longer carries any CRM query logic —
// that divergent second implementation was deleted and folded into the
// capability layer per ADR-012 §1 / §10 Increment 1.2. What remains is:
//
//   - the Reader interface the JSON-RPC layer depends on (kept as a seam so
//     the server can be unit-tested with a fake), and
//   - CapabilityReader, the production implementation that builds a
//     read-only Principal from the request workspace and dispatches to the
//     capability layer.
//
// The DB-level read-only guarantee (migration 0013's workspace_<id>_ro
// role) is preserved end-to-end: CapabilityReader builds its Principal via
// capability.MCPReadPrincipal, which sets Principal.ReadRole to the
// workspace RO role, and the capability layer's readTx issues
// `SET LOCAL ROLE workspace_<id>_ro` inside a read-only transaction. The
// pool the capability Service is built on still logs in as
// lecrm_cube_reader (ADR-009 §9), so reads can never escalate to a write.
package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/capability"
)

// ErrNotFound is returned when a single-row read matches nothing. It is an
// alias of the capability layer's sentinel so callers can keep matching on
// store.ErrNotFound while the underlying error originates in capability.
var ErrNotFound = capability.ErrNotFound

// Page is an opaque-cursor pagination request (keyset on created_at,id).
// Cursor is uuid.Nil for the first page. It mirrors capability.MCPPage so
// the MCP argument-decoding layer need not import capability directly.
type Page struct {
	Limit  int
	Cursor uuid.UUID
}

// Reader is the read surface the MCP tools depend on. The interface seam
// lets the JSON-RPC layer be unit-tested with a fake; CapabilityReader is
// the production implementation, exercised against a real Postgres by the
// capability layer's integration tests.
type Reader interface {
	ReadContact(ctx context.Context, ws, id uuid.UUID) (capability.MCPContact, error)
	ListContacts(ctx context.Context, ws uuid.UUID, p Page) (capability.MCPContacts, error)
	ReadDeal(ctx context.Context, ws, id uuid.UUID) (capability.MCPDeal, error)
	ListDeals(ctx context.Context, ws uuid.UUID, p Page) (capability.MCPDeals, error)
	ListPipelineStages(ctx context.Context, ws uuid.UUID) ([]capability.MCPStage, error)
	SearchContacts(ctx context.Context, ws uuid.UUID, query string) ([]capability.MCPContact, error)
}

// CapabilityReader implements Reader by dispatching to the shared
// capability layer. Svc must be built on a pool that logs in as the
// constrained reader role (lecrm_cube_reader); the per-workspace RO role
// is assumed per read transaction by the Principal this adapter builds.
type CapabilityReader struct {
	Svc *capability.Service
}

func (r *CapabilityReader) principal(ws uuid.UUID) capability.Principal {
	return capability.MCPReadPrincipal(ws)
}

func (r *CapabilityReader) ReadContact(ctx context.Context, ws, id uuid.UUID) (capability.MCPContact, error) {
	return r.Svc.MCPReadContact(ctx, r.principal(ws), id)
}

func (r *CapabilityReader) ListContacts(ctx context.Context, ws uuid.UUID, p Page) (capability.MCPContacts, error) {
	return r.Svc.MCPListContacts(ctx, r.principal(ws), capability.MCPPage{Limit: p.Limit, Cursor: p.Cursor})
}

func (r *CapabilityReader) ReadDeal(ctx context.Context, ws, id uuid.UUID) (capability.MCPDeal, error) {
	return r.Svc.MCPReadDeal(ctx, r.principal(ws), id)
}

func (r *CapabilityReader) ListDeals(ctx context.Context, ws uuid.UUID, p Page) (capability.MCPDeals, error) {
	return r.Svc.MCPListDeals(ctx, r.principal(ws), capability.MCPPage{Limit: p.Limit, Cursor: p.Cursor})
}

func (r *CapabilityReader) ListPipelineStages(ctx context.Context, ws uuid.UUID) ([]capability.MCPStage, error) {
	return r.Svc.MCPListPipelineStages(ctx, r.principal(ws))
}

func (r *CapabilityReader) SearchContacts(ctx context.Context, ws uuid.UUID, query string) ([]capability.MCPContact, error) {
	return r.Svc.MCPSearchContacts(ctx, r.principal(ws), query)
}
