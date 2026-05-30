// Package integrator implements the `lecrm-admin integrator` subcommands:
// grant, revoke, and list integrator access to a workspace.
//
// An integrator grant is an EMAIL-KEYED, pending authorization stored in
// core.integrator_grants (migration 0018). It records "this email may
// administrate workspace X as a hidden, non-billable integrator" decoupled
// from whether that human has ever logged in. Login-time elevation (a later
// slice) materializes a real core.workspace_members row from the grant.
//
// Provisioning auto-grants the --owner-email (see internal/tenant.Create);
// these commands cover the tenants the integrator did NOT provision.
package integrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant"
)

// GrantOptions is the parsed flag set for `integrator grant`.
type GrantOptions struct {
	Slug      string
	Email     string
	GrantedBy string // operator attribution (LECRM_OPERATOR_EMAIL or --granted-by)
}

// RevokeOptions is the parsed flag set for `integrator revoke`.
type RevokeOptions struct {
	Slug  string
	Email string
}

// ListOptions is the parsed flag set for `integrator list`. Slug is
// optional: empty lists every grant across all workspaces.
type ListOptions struct {
	Slug string
}

// Grant upserts an integrator grant for (slug, email). Idempotent: re-granting
// an existing (workspace, email) pair is a no-op.
func Grant(ctx context.Context, conn *pgx.Conn, opts GrantOptions, stdout io.Writer) error {
	if err := tenant.ValidateSlug(opts.Slug); err != nil {
		return err
	}
	if opts.Email == "" {
		return tenant.New(tenant.ErrKindSlugInvalid, "integrator grant requires --email")
	}
	id, err := resolveWorkspaceID(ctx, conn, opts.Slug)
	if err != nil {
		return err
	}
	if err := tenant.InsertIntegratorGrant(ctx, conn, id, opts.Email, opts.GrantedBy); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "integrator grant: %s → %s (workspace %s)\n", opts.Email, opts.Slug, id)
	return nil
}

// Revoke removes integrator access for (slug, email), case-insensitively on
// email. It is the feature's "off switch", so it must turn off access for the
// COMMON case — an integrator who has already logged in. That requires
// deleting TWO things, not one:
//
//  1. The pending grant in core.integrator_grants (stops any FUTURE
//     login-time elevation), and
//  2. Any already-materialized core.workspace_members row with
//     role='integrator' for the user(s) behind that email (removes LIVE
//     owner-equivalent access). Without this second delete, login elevation
//     would have already written a real membership row that is independent of
//     the grant and untouched by deleting the grant alone — leaving the
//     integrator with hidden, owner-equivalent access until someone manually
//     deleted the row.
//
// Both deletes run in a single transaction so revoke is all-or-nothing. The
// membership delete is scoped to role='integrator' so it can never remove a
// genuine owner/admin/member row that happens to share the email (email is a
// non-unique claim in core.users). Access is removed effective immediately:
// the rbac middleware resolves the role per request via LookupRole, so the
// next request after the row is gone resolves to no principal — no separate
// session revocation is needed (and a user-wide session revoke would wrongly
// log the integrator out of every other workspace too).
//
// Revoking when nothing exists is not an error — it prints a "nothing to
// revoke" notice and exits zero so scripted revokes are idempotent.
func Revoke(ctx context.Context, conn *pgx.Conn, opts RevokeOptions, stdout io.Writer) error {
	if err := tenant.ValidateSlug(opts.Slug); err != nil {
		return err
	}
	if opts.Email == "" {
		return tenant.New(tenant.ErrKindSlugInvalid, "integrator revoke requires --email")
	}
	id, err := resolveWorkspaceID(ctx, conn, opts.Slug)
	if err != nil {
		return err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return tenant.New(tenant.ErrKindDBProvision, "begin revoke tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	grantTag, err := tx.Exec(ctx,
		`DELETE FROM core.integrator_grants WHERE workspace_id = $1 AND lower(email) = lower($2)`,
		id, opts.Email)
	if err != nil {
		return tenant.New(tenant.ErrKindDBProvision, "revoke integrator grant for %q: %v", opts.Email, err)
	}

	// Also clear any materialized integrator membership so a previously
	// logged-in integrator loses live access, not just future elevation.
	memberTag, err := tx.Exec(ctx,
		`DELETE FROM core.workspace_members
		  WHERE workspace_id = $1
		    AND role = 'integrator'
		    AND user_id IN (SELECT id FROM core.users WHERE lower(email) = lower($2))`,
		id, opts.Email)
	if err != nil {
		return tenant.New(tenant.ErrKindDBProvision, "revoke integrator membership for %q: %v", opts.Email, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return tenant.New(tenant.ErrKindDBProvision, "commit revoke tx: %v", err)
	}

	if grantTag.RowsAffected() == 0 && memberTag.RowsAffected() == 0 {
		_, _ = fmt.Fprintf(stdout, "no integrator access for %s on %s (nothing to revoke)\n", opts.Email, opts.Slug)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "integrator access revoked: %s on %s (grant removed, %d live membership(s) cleared)\n",
		opts.Email, opts.Slug, memberTag.RowsAffected())
	return nil
}

// grantRow is one row of `integrator list` output.
type grantRow struct {
	Slug      string
	Email     string
	GrantedBy string
	GrantedAt time.Time
}

// List prints all integrator grants, optionally filtered to a single slug,
// joined to core.workspaces.slug for readability.
func List(ctx context.Context, conn *pgx.Conn, opts ListOptions, stdout io.Writer) error {
	var (
		rows pgx.Rows
		err  error
	)
	if opts.Slug != "" {
		if err := tenant.ValidateSlug(opts.Slug); err != nil {
			return err
		}
		// Resolve first so an unknown slug is a loud tenant_not_found rather
		// than a silently-empty list.
		if _, err := resolveWorkspaceID(ctx, conn, opts.Slug); err != nil {
			return err
		}
		rows, err = conn.Query(ctx,
			`SELECT w.slug, g.email, g.granted_by, g.granted_at
			   FROM core.integrator_grants g
			   JOIN core.workspaces w ON w.id = g.workspace_id
			  WHERE w.slug = $1
			  ORDER BY g.granted_at`, opts.Slug)
	} else {
		rows, err = conn.Query(ctx,
			`SELECT w.slug, g.email, g.granted_by, g.granted_at
			   FROM core.integrator_grants g
			   JOIN core.workspaces w ON w.id = g.workspace_id
			  ORDER BY w.slug, g.granted_at`)
	}
	if err != nil {
		return tenant.New(tenant.ErrKindDBProvision, "list integrator grants: %v", err)
	}
	defer rows.Close()

	var grants []grantRow
	for rows.Next() {
		var r grantRow
		if err := rows.Scan(&r.Slug, &r.Email, &r.GrantedBy, &r.GrantedAt); err != nil {
			return tenant.New(tenant.ErrKindDBProvision, "scan integrator grant: %v", err)
		}
		grants = append(grants, r)
	}
	if err := rows.Err(); err != nil {
		return tenant.New(tenant.ErrKindDBProvision, "iterate integrator grants: %v", err)
	}

	if len(grants) == 0 {
		_, _ = fmt.Fprintln(stdout, "no integrator grants")
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SLUG\tEMAIL\tGRANTED_BY\tGRANTED_AT")
	for _, g := range grants {
		grantedBy := g.GrantedBy
		if grantedBy == "" {
			grantedBy = "(system)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			g.Slug, g.Email, grantedBy, g.GrantedAt.UTC().Format(time.RFC3339))
	}
	return tw.Flush()
}

// resolveWorkspaceID maps a (non-tombstoned) slug to its workspace UUID,
// returning a tenant_not_found StructErr when the slug is unknown.
func resolveWorkspaceID(ctx context.Context, conn *pgx.Conn, slug string) (uuid.UUID, error) {
	var id uuid.UUID
	err := conn.QueryRow(ctx,
		`SELECT id FROM core.workspaces WHERE slug = $1 AND tombstoned_at IS NULL`, slug).Scan(&id)
	switch {
	case err == nil:
		return id, nil
	case errors.Is(err, pgx.ErrNoRows):
		return uuid.Nil, tenant.New(tenant.ErrKindTenantNotFound,
			"no active tenant with slug %q", slug)
	default:
		return uuid.Nil, tenant.New(tenant.ErrKindDBConnect, "lookup workspace by slug: %v", err)
	}
}
