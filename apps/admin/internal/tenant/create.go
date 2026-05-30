// Package tenant implements the lecrm-admin tenant subcommands.
package tenant

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// slugRegex enforces AC-V1: lowercase ASCII start, alphanumeric + hyphens,
// length 3-32. Anchored, no Unicode escapes — exactly what Léo can type.
var slugRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

// ValidateSlug returns nil if slug satisfies AC-V1, or a StructErr of kind
// slug_invalid otherwise. Call this BEFORE opening a DB connection so an
// invalid slug never produces a constraint-violation surface error.
func ValidateSlug(slug string) error {
	if !slugRegex.MatchString(slug) {
		return New(ErrKindSlugInvalid,
			"slug %q does not match the required pattern %s "+
				"(lowercase ASCII start, alphanumeric + hyphens, length 3-32)",
			slug, slugRegex.String())
	}
	return nil
}

// CreateOptions is the parsed flag set for tenant create. The CLI layer
// (cmd/lecrm-admin) populates this from urfave/cli context.
type CreateOptions struct {
	Slug          string
	AdminEmail    string
	OwnerEmail    string // CLI alias: creator_email — Léo's identity
	OperatorEmail string // who ran the provision (LECRM_OPERATOR_EMAIL); grant attribution
	DisplayName   string // unused in 8.1; reserved for v1 (no DB column yet)
	Template      string
	ForceRecreate bool
	Upsert        bool
}

// CreateResult is what Create returns on success. The Stdout writer
// receives one machine-parseable line `WORKSPACE_ID=<uuid>` so CI can
// capture the ID for cleanup (T5 smoke test).
type CreateResult struct {
	WorkspaceID uuid.UUID
	Slug        string
	RoleName    string
}

// Create provisions a tenant by calling
// core.lecrm_provision_workspace_with_registry. Behavior:
//
//   - default: pre-check core.workspaces by slug; if present, fail loud
//     with AC-F2 verbatim error
//   - --upsert: look up existing UUID by slug (if any) and pass it to
//     the wrapper so ON CONFLICT (id) DO NOTHING is a true no-op
//   - --force-recreate: in one transaction, drop the old workspace
//     (audit rows → workspaces row → river schema → workspace schema →
//     role) then call the wrapper with a fresh UUIDv7
//
// The Postgres txn boundary is the atomicity guarantee for AC-T1 and
// AC-F4. Slug validation runs before the connection is opened.
func Create(ctx context.Context, conn *pgx.Conn, opts CreateOptions, stdout io.Writer) (CreateResult, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return CreateResult{}, err
	}

	creatorEmail := opts.OwnerEmail
	if creatorEmail == "" {
		creatorEmail = opts.AdminEmail
	}
	template := opts.Template
	if template == "" {
		template = templates.GBConsultDefaultName
	}

	// Auto-grant integrator access only when --owner-email was explicitly
	// passed. The integrator is the owner-email (Léo); when it is absent
	// creatorEmail falls back to the CLIENT's admin-email, who is a normal
	// owner and must NOT be turned into a hidden, non-billable integrator.
	integratorEmail := opts.OwnerEmail
	grantedBy := opts.OperatorEmail

	switch {
	case opts.ForceRecreate:
		return createForceRecreate(ctx, conn, opts.Slug, opts.AdminEmail, creatorEmail, integratorEmail, grantedBy, template, stdout)
	case opts.Upsert:
		return createUpsert(ctx, conn, opts.Slug, opts.AdminEmail, creatorEmail, integratorEmail, grantedBy, template, stdout)
	default:
		return createFresh(ctx, conn, opts.Slug, opts.AdminEmail, creatorEmail, integratorEmail, grantedBy, template, stdout)
	}
}

// createFresh is the default path. Fails loud (AC-F2) if the slug is
// already in core.workspaces. Also rejects reserved and tombstoned slugs
// to prevent subdomain takeover (council-architecture-review-2026-05-24).
func createFresh(ctx context.Context, conn *pgx.Conn, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template string, stdout io.Writer) (CreateResult, error) {
	if err := checkSlugBlocked(ctx, conn, slug); err != nil {
		return CreateResult{}, err
	}

	var existingCreatedAt time.Time
	var existingCreatorEmail string
	err := conn.QueryRow(ctx,
		`SELECT created_at, creator_email FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NULL`, slug).
		Scan(&existingCreatedAt, &existingCreatorEmail)
	switch {
	case err == nil:
		return CreateResult{}, New(ErrKindSlugConflict,
			"%s %q already exists, created %s by %s. Use a different slug or pass --force-recreate (destroys data).",
			OperatorNoun, slug,
			existingCreatedAt.UTC().Format(time.RFC3339),
			fallback(existingCreatorEmail, "unknown"))
	case errors.Is(err, pgx.ErrNoRows):
		// fall through
	default:
		return CreateResult{}, New(ErrKindDBConnect, "lookup workspace by slug: %v", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return CreateResult{}, New(ErrKindDBProvision, "mint UUIDv7: %v", err)
	}

	return callWrapper(ctx, conn, id, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template, stdout)
}

// createUpsert looks up the existing UUID by slug (if any) and re-runs
// the wrapper. The wrapper's ON CONFLICT (id) DO NOTHING path keeps DB
// state bit-identical (AC-F3 / AC-I-10).
func createUpsert(ctx context.Context, conn *pgx.Conn, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template string, stdout io.Writer) (CreateResult, error) {
	if err := checkSlugBlocked(ctx, conn, slug); err != nil {
		return CreateResult{}, err
	}

	var existingID uuid.UUID
	err := conn.QueryRow(ctx,
		`SELECT id FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NULL`, slug).Scan(&existingID)
	switch {
	case err == nil:
		// Existing workspace: re-run wrapper with the same UUID; ON CONFLICT
		// (id) DO NOTHING preserves state.
		return callWrapper(ctx, conn, existingID, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template, stdout)
	case errors.Is(err, pgx.ErrNoRows):
		// Fresh provision via the --upsert flag.
		id, err := uuid.NewV7()
		if err != nil {
			return CreateResult{}, New(ErrKindDBProvision, "mint UUIDv7: %v", err)
		}
		return callWrapper(ctx, conn, id, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template, stdout)
	default:
		return CreateResult{}, New(ErrKindDBConnect, "lookup workspace by slug: %v", err)
	}
}

// createForceRecreate destroys the old workspace (audit rows + registry +
// schemas + role) inside a single transaction, then provisions a fresh
// UUIDv7 via the wrapper in the SAME transaction. If anything fails, the
// txn rolls back and the original tenant survives intact.
func createForceRecreate(ctx context.Context, conn *pgx.Conn, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template string, stdout io.Writer) (CreateResult, error) {
	if err := checkSlugBlocked(ctx, conn, slug); err != nil {
		return CreateResult{}, err
	}

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CreateResult{}, New(ErrKindDBConnect, "begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingID uuid.UUID
	var existingRoleName string
	err = tx.QueryRow(ctx,
		`SELECT id, role_name FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NULL`, slug).
		Scan(&existingID, &existingRoleName)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Nothing to drop; fall through to fresh provision.
	case err != nil:
		return CreateResult{}, New(ErrKindDBConnect, "lookup workspace by slug: %v", err)
	default:
		if err := dropExistingWorkspace(ctx, tx, existingID, existingRoleName); err != nil {
			return CreateResult{}, err
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return CreateResult{}, New(ErrKindDBProvision, "mint UUIDv7: %v", err)
	}

	var roleName string
	if err := tx.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		id, slug, adminEmail, creatorEmail, template).Scan(&roleName); err != nil {
		return CreateResult{}, New(ErrKindDBProvision, "provision wrapper: %v", err)
	}

	// Auto-grant the integrator inside the SAME transaction as provisioning
	// (AC-T1 atomicity): either the workspace and its grant both commit, or
	// neither does.
	if integratorEmail != "" {
		if err := InsertIntegratorGrant(ctx, tx, id, integratorEmail, grantedBy); err != nil {
			return CreateResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CreateResult{}, New(ErrKindDBProvision, "commit tx: %v", err)
	}

	if integratorEmail != "" {
		_, _ = fmt.Fprintf(stdout, "[PROVISION] integrator grant: %s\n", integratorEmail)
	}
	emitResult(stdout, id, slug, roleName)
	return CreateResult{WorkspaceID: id, Slug: slug, RoleName: roleName}, nil
}

// dropExistingWorkspace removes everything Story 8.1 created for a tenant.
// Order matters: audit rows reference workspaces(id), so they go first; the
// workspaces row goes before the role/schemas because the FK is the only
// hard constraint. DROP SCHEMA CASCADE + DROP ROLE IF EXISTS handle the
// rest. All of this runs inside the caller's transaction.
func dropExistingWorkspace(ctx context.Context, tx pgx.Tx, id uuid.UUID, roleName string) error {
	riverSchema := "river_" + uuidNoHyphens(id)

	if _, err := tx.Exec(ctx, `DELETE FROM core.audit_log WHERE workspace_id = $1`, id); err != nil {
		return New(ErrKindDBProvision, "drop audit rows: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM core.workspaces WHERE id = $1`, id); err != nil {
		return New(ErrKindDBProvision, "drop workspaces row: %v", err)
	}
	// Schema and role drops use identifier interpolation (fmt.Sprintf with
	// pgx-quoted names). We trust the role_name from core.workspaces since
	// it was generated by lecrm_provision_workspace, not user input.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, pgxIdent(roleName))); err != nil {
		return New(ErrKindDBProvision, "drop workspace schema: %v", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, pgxIdent(riverSchema))); err != nil {
		return New(ErrKindDBProvision, "drop river schema: %v", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS %s`, pgxIdent(roleName))); err != nil {
		return New(ErrKindDBProvision, "drop workspace role: %v", err)
	}
	return nil
}

// callWrapper invokes the SECURITY DEFINER wrapper for the non-destructive
// paths (createFresh, createUpsert) and emits the structured output line.
func callWrapper(ctx context.Context, conn *pgx.Conn, id uuid.UUID, slug, adminEmail, creatorEmail, integratorEmail, grantedBy, template string, stdout io.Writer) (CreateResult, error) {
	var roleName string
	if err := conn.QueryRow(ctx,
		`SELECT core.lecrm_provision_workspace_with_registry($1, $2, $3, $4, $5)`,
		id, slug, adminEmail, creatorEmail, template).Scan(&roleName); err != nil {
		return CreateResult{}, New(ErrKindDBProvision, "provision wrapper: %v", err)
	}

	// Auto-grant the integrator on the same connection right after the
	// wrapper committed. ON CONFLICT DO NOTHING keeps --upsert re-runs
	// idempotent (re-granting an existing grant is a no-op).
	if integratorEmail != "" {
		if err := InsertIntegratorGrant(ctx, conn, id, integratorEmail, grantedBy); err != nil {
			return CreateResult{}, err
		}
		_, _ = fmt.Fprintf(stdout, "[PROVISION] integrator grant: %s\n", integratorEmail)
	}

	emitResult(stdout, id, slug, roleName)
	return CreateResult{WorkspaceID: id, Slug: slug, RoleName: roleName}, nil
}

// emitResult writes the operator-facing success line plus the
// machine-parseable WORKSPACE_ID=<uuid> line that CI's smoke test parses
// for the cleanup step (T5).
func emitResult(stdout io.Writer, id uuid.UUID, slug, roleName string) {
	_, _ = fmt.Fprintf(stdout, "%s %q provisioned. role=%s\n", OperatorNoun, slug, roleName)
	_, _ = fmt.Fprintf(stdout, "WORKSPACE_ID=%s\n", id)
	// AC-I-13 / T3 — RBAC seeding status line. Written to stdout, NOT to
	// core.audit_log. Replaces with `ok (N roles applied)` when RBAC ships.
	// TODO(rbac): swap to `[PROVISION] RBAC seeding: ok (3 roles applied)`
	// once the RBAC-seeding sibling story lands.
	_, _ = fmt.Fprintln(stdout, "[PROVISION] RBAC seeding: skipped (not implemented in v0)")
}

// uuidNoHyphens returns the lowercase hex form expected by the SQL
// function (workspace_<32hex>, river_<32hex>).
func uuidNoHyphens(id uuid.UUID) string {
	b := id.String()
	out := make([]byte, 0, 32)
	for i := 0; i < len(b); i++ {
		if b[i] != '-' {
			out = append(out, b[i])
		}
	}
	return string(out)
}

// pgxIdent quotes a Postgres identifier. Used only for trusted names
// (role_name) read back from core.workspaces, never user input.
func pgxIdent(name string) string {
	// Reject anything outside [a-z0-9_] to be defense-in-depth even though
	// the only producer is the SECURITY DEFINER function.
	for i := 0; i < len(name); i++ {
		c := name[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLower && !isDigit && c != '_' {
			return `"invalid_identifier"`
		}
	}
	return `"` + name + `"`
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// checkSlugBlocked rejects slugs that are reserved (infrastructure names)
// or tombstoned (previously-deleted tenants). This is the subdomain takeover
// prevention gate from council-architecture-review-2026-05-24.
func checkSlugBlocked(ctx context.Context, conn *pgx.Conn, slug string) error {
	var reserved bool
	err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM core.reserved_slugs WHERE slug = $1)`, slug).Scan(&reserved)
	if err != nil {
		return New(ErrKindDBConnect, "check reserved slugs: %v", err)
	}
	if reserved {
		return New(ErrKindSlugReserved,
			"slug %q is reserved (infrastructure name) and cannot be provisioned", slug)
	}

	var tombstoned bool
	err = conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NOT NULL)`,
		slug).Scan(&tombstoned)
	if err != nil {
		return New(ErrKindDBConnect, "check tombstoned slugs: %v", err)
	}
	if tombstoned {
		return New(ErrKindSlugTombstoned,
			"slug %q was previously used by a deleted tenant and cannot be re-provisioned (subdomain takeover prevention)",
			slug)
	}
	return nil
}
