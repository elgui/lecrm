// Package capability is the protocol-agnostic CRM capability layer
// (ADR-012 §1). It is the single source of CRM business logic: every
// operation enforces RBAC, idempotency (core.idempotency_keys, ADR-007),
// and fail-closed audit (core.audit_log, ADR-007 §7.2) *inside* the call,
// and returns domain results — never wire formats.
//
// It depends only on the store (sqlcgen + raw pgx) and domain validation.
// It never imports the HTTP (chi) transport or the JSON-RPC layer, and defines
// its own Principal type rather than importing apps/api/internal/rbac so
// that no transport package leaks into it. REST handlers, connector-event
// handlers (apps/api), and the MCP adapter (apps/mcp) are all thin
// projections that build a Principal and call into this layer.
//
// Location note: this package lives in the apps/api module (a *non-internal*
// package) rather than packages/crm-adapter/. That keeps go.work unchanged
// and lets it reuse apps/api/internal/sqlcgen verbatim — guaranteeing zero
// SQL drift from the previous HTTP-coupled implementation — while remaining
// importable by apps/mcp (which only needs `require .../apps/api`). See
// ADR-012 §1 / TO RESOLVE 1.
package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultPageLimit is the keyset page size for list operations. It mirrors
// the value the REST handlers used before extraction.
const DefaultPageLimit int32 = 50

// idempotencyTTL is the lifetime of a cached response for one
// (workspace_id, key) pair (ADR-007 / ADR-009 §4: 24h).
const idempotencyTTL = 24 * time.Hour

// --- Role / Principal ---

// Role is a workspace membership level. The integer ordering matches
// apps/api/internal/rbac.Role (None<Member<Admin<Owner) so the REST
// adapter can translate by value, but this type is defined here to keep
// the capability layer free of the rbac package (which imports net/http).
type Role int

const (
	// RoleNone is the absence of a workspace role; it satisfies no gate.
	RoleNone Role = iota
	// RoleMember can read all CRM entities.
	RoleMember
	// RoleAdmin can read and mutate CRM entities.
	RoleAdmin
	// RoleOwner can do everything admin can, plus workspace administration.
	RoleOwner
)

// AtLeast reports whether r is at or above min. RoleNone never satisfies a
// gate.
func (r Role) AtLeast(min Role) bool { return r >= min && r != RoleNone }

// Principal is the resolved authorization identity a capability operation
// acts on behalf of. The adapter resolves it once (from a session cookie,
// a service token, or a connector token) and hands it to every call.
//
//   - WorkspaceID / Schema scope the data: Schema is the per-workspace
//     Postgres role/schema the transaction pins its search_path to
//     (workspace.Context.RoleName).
//   - Role drives RBAC (reads require RoleMember+, writes RoleAdmin+).
//   - ActorType is recorded on every audit row (ADR-009 §7.2).
//   - ReadRole optionally names a constrained Postgres role to `SET LOCAL
//     ROLE` to for the lifetime of a *read* transaction. When non-empty,
//     readTx assumes it before pinning search_path, so the database — not
//     the Go code — enforces SELECT-only access. This is how a read-only
//     adapter (the MCP binary, ADR-009 §9) preserves the DB-level
//     read-only guarantee: it connects as `lecrm_cube_reader` and sets
//     ReadRole to the workspace's `workspace_<id>_ro` role (migration
//     0013). The REST/connector callers leave it empty (their pool login
//     role already carries the right privileges), so their behaviour is
//     unchanged. ReadRole is ignored by writeTx — write paths must use a
//     pool whose login role can mutate.
type Principal struct {
	WorkspaceID    uuid.UUID
	Schema         string
	Role           Role
	ActorType      string
	Scopes         []string
	IsServiceToken bool
	ReadRole       string
}

// --- typed errors (adapter maps these to transport status codes) ---

var (
	// ErrUnauthenticated means no usable principal (RoleNone). REST → 401.
	ErrUnauthenticated = errors.New("authentication required")
	// ErrForbidden means the principal's role is below the operation's
	// minimum. REST → 403.
	ErrForbidden = errors.New("insufficient role")
	// ErrNotFound means a single-row read/mutation matched nothing.
	// REST → 404.
	ErrNotFound = errors.New("not found")
)

// ValidationError is a 400-class domain validation failure. The Msg is
// safe to surface to the client (it mirrors the domain validators).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func validationErr(err error) error {
	if err == nil {
		return nil
	}
	return &ValidationError{Msg: err.Error()}
}

// authorize enforces the role gate for an operation. RoleNone →
// ErrUnauthenticated, a present-but-insufficient role → ErrForbidden.
func authorize(p Principal, min Role) error {
	if p.Role == RoleNone {
		return ErrUnauthenticated
	}
	if !p.Role.AtLeast(min) {
		return ErrForbidden
	}
	return nil
}

// Service is the capability layer's entry point. It holds the long-lived
// pgx pool and logger; all per-request state arrives via the Principal.
type Service struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

// New constructs a Service.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Service {
	return &Service{Pool: pool, Logger: logger}
}

// --- transaction wrappers (workspace-scoped) ---

// ReadTx runs fn inside a read-only transaction whose search_path is pinned
// to schema. Exported so the remaining thin HTTP handlers in
// apps/api/internal/crm (notes/tasks/CSV export) share exactly one tx
// implementation.
func ReadTx(ctx context.Context, pool *pgxpool.Pool, schema string, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WriteTx runs fn inside a read-write transaction whose search_path is
// pinned to schema. The fail-closed audit invariant (ADR-009 §7.2) is a
// property of this wrapper: any error fn returns — including a failed audit
// insert — rolls the whole transaction back.
func WriteTx(ctx context.Context, pool *pgxpool.Pool, schema string, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReadTxAsRole is ReadTx plus a `SET LOCAL ROLE role` issued before the
// search_path is pinned, so the constrained role is the effective
// principal for the transaction. Both SET LOCAL statements revert on
// commit/rollback, leaving no role/search_path leakage across pooled
// connections. The explicit search_path is still required: SET ROLE
// changes privileges but does not apply the target role's login-time
// ALTER ROLE search_path, so unqualified names would otherwise resolve
// against the login role's default path. (Mirrors the old
// apps/mcp/internal/store.withWorkspace, now folded in here per ADR-012 §1.)
func ReadTxAsRole(ctx context.Context, pool *pgxpool.Pool, role, schema string, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
		return fmt.Errorf("set role: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return fmt.Errorf("set search_path: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) readTx(ctx context.Context, p Principal, fn func(pgx.Tx) error) error {
	if p.ReadRole != "" {
		return ReadTxAsRole(ctx, s.Pool, p.ReadRole, p.Schema, fn)
	}
	return ReadTx(ctx, s.Pool, p.Schema, fn)
}

func (s *Service) writeTx(ctx context.Context, p Principal, fn func(pgx.Tx) error) error {
	return WriteTx(ctx, s.Pool, p.Schema, fn)
}

// --- audit / activity emission (fail-closed, inside the caller's tx) ---

// EmitAudit writes one row into core.audit_log inside tx. Fail-closed: when
// it errors the surrounding WriteTx rolls back and the mutation is rejected
// (ADR-009 §7.2). actorType is supplied by the caller (the Principal's
// actor type for capability ops; ctx-derived for the connector path).
func EmitAudit(ctx context.Context, tx pgx.Tx, event string, workspaceID uuid.UUID, actorType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("audit marshal %s: %w", event, err)
	}
	if actorType == "" {
		actorType = ActorTypeHumanAPI
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO core.audit_log (event, workspace_id, actor_type, payload)
		 VALUES ($1, $2, $3, $4)`,
		// string(body), not body: under pgx's simple query protocol a []byte
		// is sent as a bytea literal and rejected by the jsonb column (22P02).
		event, workspaceID, actorType, string(body),
	); err != nil {
		return fmt.Errorf("audit insert %s: %w", event, err)
	}
	return nil
}

// Actor types — must match the core.audit_log actor_type CHECK constraint
// (defined in 0001_init.sql, extended by 0019 for 'integrator' and 0023 for
// 'connector') and the per-workspace activities CHECK (0015 / 0022).
const (
	ActorTypeHumanAPI        = "human_api"
	ActorTypeMCPAgent        = "mcp_agent"
	ActorTypeInternalService = "internal_service"
	ActorTypeSystem          = "system"
	// ActorTypeConnector tags writes performed by the connector ingestion path
	// (POST /v1/connectors/{source}/events). It is admitted by the per-workspace
	// activities CHECK (0015 / 0022) and — as of migration 0023 — by the
	// core.audit_log actor_type CHECK; before 0023 every connector EmitAudit
	// call rolled back fail-closed (SQLSTATE 23514).
	ActorTypeConnector = "connector"
	// ActorTypeIntegrator tags writes performed by GB Consult's integrator
	// principal in the canonical audit trail (core.audit_log). Migration 0019
	// extends the core.audit_log actor_type CHECK to admit it. The
	// per-workspace activities table CHECK predates the integrator role and is
	// NOT migrated per-schema; emitEntityActivity therefore maps integrator →
	// human_api for that entity timeline (the security-relevant attribution
	// lives in core.audit_log).
	ActorTypeIntegrator = "integrator"
)

// Entity types — must match the CHECK constraint in migration 0015.
const (
	EntityTypeContact = "contact"
	EntityTypeCompany = "company"
	EntityTypeDeal    = "deal"
)

// EmitActivity writes one row into the workspace's activities table inside
// tx (search_path already pinned). Fail-closed, same contract as EmitAudit.
func EmitActivity(ctx context.Context, tx pgx.Tx, entityType string, entityID uuid.UUID, eventType, actorType, sourceSystem string, payload any) error {
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

// emitEntityActivity emits an activity attributed to the principal's actor
// type (the capability-op default; equivalent to the old emitRESTActivity
// for human_api callers).
func (s *Service) emitEntityActivity(ctx context.Context, tx pgx.Tx, p Principal, entityType string, entityID uuid.UUID, eventType string, payload any) error {
	actor := p.ActorType
	if actor == "" {
		actor = ActorTypeHumanAPI
	}
	// The per-workspace activities table's actor_type CHECK does not include
	// 'integrator' (it is not migrated per-schema). The canonical integrator
	// attribution is recorded in core.audit_log via EmitAudit; for the entity
	// timeline, map integrator → human_api so the fail-closed write succeeds.
	if actor == ActorTypeIntegrator {
		actor = ActorTypeHumanAPI
	}
	return EmitActivity(ctx, tx, entityType, entityID, eventType, actor, "", payload)
}

// --- idempotency (core.idempotency_keys, ADR-007) ---

// IdempotencyLookup checks for a cached response for (workspace, key). It
// runs outside any mutation transaction so a hit short-circuits before a
// write tx is opened.
func IdempotencyLookup(ctx context.Context, pool *pgxpool.Pool, ws uuid.UUID, key string) (int, []byte, bool, error) {
	var status int
	var body []byte
	err := pool.QueryRow(ctx,
		`SELECT response_status, response_body
		   FROM core.idempotency_keys
		  WHERE workspace_id = $1 AND key = $2 AND expires_at > now()`,
		ws, key,
	).Scan(&status, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	return status, body, true, nil
}

// IdempotencyStore persists a captured response inside the caller's tx, so
// the mutation + audit + key insert commit atomically. ON CONFLICT DO
// NOTHING lets the first committer win a duplicate race.
func IdempotencyStore(ctx context.Context, tx pgx.Tx, ws uuid.UUID, key, method, path string, status int, body []byte) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO core.idempotency_keys
		   (key, workspace_id, method, path, response_status, response_body, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now() + $7::interval)
		 ON CONFLICT (workspace_id, key) DO NOTHING`,
		key, ws, method, path, status, body, idempotencyTTL.String(),
	)
	return err
}

// MutationResult is the outcome of an idempotent create. Body is the
// canonical JSON of the created (or replayed) domain object — identical
// bytes on a fresh execution and on a replay, so the transport adapter can
// emit it verbatim. Replayed flags a cache hit.
type MutationResult struct {
	Status   int
	Body     []byte
	Replayed bool
}

// --- custom properties (ADR-010 objects-table storage) ---

// MergeCustomProps merges props into the existing custom_properties JSONB
// bag for (parentType, parentID), inside the caller's tx.
func MergeCustomProps(ctx context.Context, tx pgx.Tx, parentType string, parentID uuid.UUID, props map[string]any) error {
	existing := map[string]any{}
	var raw []byte
	err := tx.QueryRow(ctx,
		`SELECT data FROM objects
		  WHERE object_type = 'custom_properties' AND parent_type = $1 AND parent_id = $2`,
		parentType, parentID).Scan(&raw)
	if err == nil {
		if e := json.Unmarshal(raw, &existing); e != nil {
			return e
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	for k, v := range props {
		existing[k] = v
	}
	merged, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM objects WHERE object_type = 'custom_properties' AND parent_type = $1 AND parent_id = $2`,
		parentType, parentID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO objects (object_type, parent_type, parent_id, data)
		 VALUES ('custom_properties', $1, $2, $3)`,
		// string(merged), not merged: under pgx's simple query protocol a []byte
		// is sent as a bytea literal and rejected by the jsonb column (22P02).
		parentType, parentID, string(merged))
	return err
}

// --- pgtype conversion helpers (row→wire and input→param) ---

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func uuidPtr(u uuid.NullUUID) *string {
	if !u.Valid {
		return nil
	}
	s := u.UUID.String()
	return &s
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	s := d.Time.Format("2006-01-02")
	return &s
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func numPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	return &f.Float64
}

func toText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toNullUUID(s *string) uuid.NullUUID {
	if s == nil {
		return uuid.NullUUID{}
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: id, Valid: true}
}

func toNumeric(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*f, 'f', -1, 64))
	return n
}

func toDate(s *string) pgtype.Date {
	if s == nil {
		return pgtype.Date{}
	}
	t, err := time.Parse("2006-01-02", *s)
	if err != nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}
