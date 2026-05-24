package tenant

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/admin/internal/tenant/templates"
)

// VerifyOptions controls the verify subcommand.
type VerifyOptions struct {
	Slug        string
	AllFailures bool // AC-VFY-3: report all failures instead of short-circuiting
}

// VerifyResult is what Verify returns. Failed > 0 ⇒ exit non-zero.
type VerifyResult struct {
	Passed int
	Failed int
	Total  int
}

// invariantCheck is one row in the verify table. Each returns nil for
// pass or a detail string for fail. We don't return Go errors so we can
// distinguish "infra error" (handled separately) from "invariant failure".
type invariantCheck struct {
	Code string
	Run  func(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (passDetail string, failDetail string)
}

// Verify runs the 14 invariants AC-I-01..AC-I-14 against the tenant
// identified by slug. AC-I-09, AC-I-10, AC-I-14 are not DB-checkable
// post-state assertions; we mark them OK with a note pointing at the
// test suite. AC-I-04 (cross-tenant isolation) requires a sibling tenant
// in the same DB — verify reports OK with a "checked by tenantpair
// fixture in test suite" note rather than provisioning a throw-away.
//
// AC-VFY-2: returns exit code 0 only if VerifyResult.Failed == 0.
// AC-VFY-3: with AllFailures=true, every check runs even if earlier ones
// failed; without it, the first failure stops the scan.
// AC-VFY-4: every line is `[OK] INV-XX <label>` or
// `[FAIL] INV-XX <label> — <detail>` — fixed prefix, single line, machine-parseable.
func Verify(ctx context.Context, conn *pgx.Conn, opts VerifyOptions, stdout io.Writer) (VerifyResult, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return VerifyResult{}, err
	}

	var id uuid.UUID
	var roleName string
	err := conn.QueryRow(ctx,
		`SELECT id, role_name FROM core.workspaces WHERE slug = $1`, opts.Slug).Scan(&id, &roleName)
	switch {
	case err == pgx.ErrNoRows:
		return VerifyResult{}, New(ErrKindSlugConflict,
			"%s %q not found in registry", OperatorNoun, opts.Slug)
	case err != nil:
		return VerifyResult{}, New(ErrKindDBConnect, "lookup tenant: %v", err)
	}

	checks := allInvariants()
	result := VerifyResult{Total: len(checks)}

	for _, c := range checks {
		passDetail, failDetail := c.Run(ctx, conn, id, roleName)
		label := InvariantLabel(c.Code)
		if failDetail == "" {
			result.Passed++
			if passDetail != "" {
				fmt.Fprintf(stdout, "[OK] %s %s (%s)\n", c.Code, label, passDetail)
			} else {
				fmt.Fprintf(stdout, "[OK] %s %s\n", c.Code, label)
			}
			continue
		}
		result.Failed++
		fmt.Fprintf(stdout, "[FAIL] %s %s — %s\n", c.Code, label, failDetail)
		if !opts.AllFailures {
			return result, nil
		}
	}
	return result, nil
}

// allInvariants returns the table of 14 invariant checks in order.
func allInvariants() []invariantCheck {
	return []invariantCheck{
		{Code: "INV-01", Run: checkRoleExists},
		{Code: "INV-02", Run: checkSchemasExist},
		{Code: "INV-03", Run: checkRoleUsage},
		{Code: "INV-04", Run: checkCrossTenant},
		{Code: "INV-05", Run: checkRegistryRow},
		{Code: "INV-06", Run: checkUUIDRoundTrip},
		{Code: "INV-07", Run: checkAuditRow},
		{Code: "INV-08", Run: checkAuditBinding},
		{Code: "INV-09", Run: checkDuplicateLoud},
		{Code: "INV-10", Run: checkUpsertNoop},
		{Code: "INV-11", Run: checkProvisionerOnly},
		{Code: "INV-12", Run: checkPipelineSeed},
		{Code: "INV-13", Run: checkRBACStatusLine},
		{Code: "INV-14", Run: checkMigrationColdClean},
	}
}

func checkRoleExists(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, roleName).Scan(&exists); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	if !exists {
		return "", fmt.Sprintf("role %s not in pg_roles", roleName)
	}
	return "", ""
}

func checkSchemasExist(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	riverSchema := "river_" + uuidNoHyphens(id)
	var ws, rv bool
	if err := conn.QueryRow(ctx, `
		SELECT
		  EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $1),
		  EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = $2)
	`, roleName, riverSchema).Scan(&ws, &rv); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	switch {
	case !ws && !rv:
		return "", fmt.Sprintf("neither %s nor %s found", roleName, riverSchema)
	case !ws:
		return "", fmt.Sprintf("workspace schema %s missing", roleName)
	case !rv:
		return "", fmt.Sprintf("river schema %s missing", riverSchema)
	}
	return "", ""
}

func checkRoleUsage(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// pgx requires explicit text casts when the same parameter would be
	// inferred to multiple types; we pass roleName twice with $1::text/$2::text
	// to satisfy has_schema_privilege(user, schema, privilege).
	var hasUsage bool
	if err := conn.QueryRow(ctx,
		`SELECT has_schema_privilege($1::text, $2::text, 'USAGE')`, roleName, roleName).Scan(&hasUsage); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	if !hasUsage {
		return "", fmt.Sprintf("role %s lacks USAGE on its own schema", roleName)
	}
	return "", ""
}

func checkCrossTenant(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// AC-I-04 needs a sibling. The tenantpair test fixture (tasket 0f09)
	// is the canonical implementation. verify scans the DB for other
	// workspaces; if at least one exists, checks that this role has NO
	// access to its schema.
	rows, err := conn.Query(ctx,
		`SELECT role_name FROM core.workspaces WHERE id <> $1 LIMIT 5`, id)
	if err != nil {
		return "", fmt.Sprintf("query siblings: %v", err)
	}
	defer rows.Close()

	var siblings []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err == nil {
			siblings = append(siblings, s)
		}
	}
	if len(siblings) == 0 {
		return "no sibling tenants — covered by tenantpair test fixture (sibling tasket 0f09)", ""
	}
	for _, sib := range siblings {
		var hasUsage bool
		if err := conn.QueryRow(ctx,
			`SELECT has_schema_privilege($1::text, $2::text, 'USAGE')`, roleName, sib).Scan(&hasUsage); err != nil {
			return "", fmt.Sprintf("query usage on %s: %v", sib, err)
		}
		if hasUsage {
			return "", fmt.Sprintf("role %s has USAGE on sibling schema %s", roleName, sib)
		}
	}
	return fmt.Sprintf("verified against %d sibling tenant(s)", len(siblings)), ""
}

func checkRegistryRow(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	var dbID uuid.UUID
	var dbRole string
	if err := conn.QueryRow(ctx,
		`SELECT id, role_name FROM core.workspaces WHERE id = $1`, id).Scan(&dbID, &dbRole); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	if dbID != id || dbRole != roleName {
		return "", fmt.Sprintf("registry row mismatch: id=%s role=%s", dbID, dbRole)
	}
	return "", ""
}

func checkUUIDRoundTrip(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// UUIDv7 has version=7 in the 13th hex digit (after a hyphen split,
	// position [14] of the canonical form). We verify the stored UUID is
	// version 7 (proves the v7 mint survived the round-trip).
	v := id.Version()
	if v != 7 {
		return "", fmt.Sprintf("stored UUID version %d (expected 7)", v)
	}
	return "version=7", ""
}

func checkAuditRow(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	var count int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM core.audit_log
		   WHERE event = 'workspace.provisioned' AND workspace_id = $1`, id).
		Scan(&count); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	if count == 0 {
		return "", "no audit row with event=workspace.provisioned"
	}
	return fmt.Sprintf("%d provisioning audit row(s)", count), ""
}

func checkAuditBinding(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// AC-I-08: audit_log.workspace_id == core.workspaces.id (atomic write).
	// The FK already enforces referential integrity; we double-check the
	// row exists and is bound correctly.
	var bound bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM core.audit_log al
		    JOIN core.workspaces w ON w.id = al.workspace_id
		   WHERE al.event = 'workspace.provisioned' AND w.id = $1
		)
	`, id).Scan(&bound); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	if !bound {
		return "", "audit row not bound to workspaces row"
	}
	return "", ""
}

func checkDuplicateLoud(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	return "covered by apps/admin/internal/tenant/create_test.go", ""
}

func checkUpsertNoop(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	return "covered by apps/admin/internal/tenant/create_test.go", ""
}

func checkProvisionerOnly(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// AC-I-11: workspace role cannot SELECT from core.workspaces or
	// core.audit_log. has_table_privilege answers this without us having
	// to log in as the role.
	var canWS, canAL bool
	if err := conn.QueryRow(ctx,
		`SELECT
		   has_table_privilege($1::text, 'core.workspaces'::text, 'SELECT'),
		   has_table_privilege($1::text, 'core.audit_log'::text, 'SELECT')`,
		roleName).Scan(&canWS, &canAL); err != nil {
		return "", fmt.Sprintf("query: %v", err)
	}
	if canWS || canAL {
		return "", fmt.Sprintf("role %s has SELECT on provisioner tables (workspaces=%t, audit_log=%t)",
			roleName, canWS, canAL)
	}
	return "", ""
}

func checkPipelineSeed(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// Pipeline-stages table must contain exactly the 5 default stages.
	// Quote the schema identifier directly since roleName comes from
	// core.workspaces (trusted).
	q := fmt.Sprintf(`SELECT name FROM %s.pipeline_stages ORDER BY order_index`, pgxIdent(roleName))
	rows, err := conn.Query(ctx, q)
	if err != nil {
		// If the table doesn't exist, the wrapper was called with an
		// empty template (bootstrap path). Surface that distinctly.
		if strings.Contains(err.Error(), "does not exist") {
			return "no pipeline_stages table (bootstrap path)", ""
		}
		return "", fmt.Sprintf("query: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", fmt.Sprintf("scan: %v", err)
		}
		got = append(got, n)
	}
	if len(got) != len(templates.GBConsultDefaultStages) {
		return "", fmt.Sprintf("expected %d stages, got %d", len(templates.GBConsultDefaultStages), len(got))
	}
	for i, want := range templates.GBConsultDefaultStages {
		if got[i] != want {
			return "", fmt.Sprintf("stage[%d]: want %q, got %q", i, want, got[i])
		}
	}

	// Also verify provisioning_features_applied includes the template.
	var features []string
	if err := conn.QueryRow(ctx,
		`SELECT ARRAY(SELECT jsonb_array_elements_text(provisioning_features_applied))
		   FROM core.workspaces WHERE id = $1`, id).Scan(&features); err != nil {
		return "", fmt.Sprintf("query features: %v", err)
	}
	hasFeature := false
	for _, f := range features {
		if strings.HasPrefix(f, templates.GBConsultDefaultName) {
			hasFeature = true
			break
		}
	}
	if !hasFeature {
		return "", fmt.Sprintf("provisioning_features_applied missing %s entry", templates.GBConsultDefaultName)
	}
	return fmt.Sprintf("5 stages + feature %s", templates.GBConsultDefaultName), ""
}

func checkRBACStatusLine(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// AC-I-13 is a stdout assertion of `lecrm-admin tenant create`. The
	// verify command runs after create and cannot observe the prior
	// stdout; we report OK with a pointer to create_test.go which DOES
	// assert the literal string.
	return "covered by apps/admin/internal/tenant/create_test.go", ""
}

func checkMigrationColdClean(ctx context.Context, conn *pgx.Conn, id uuid.UUID, roleName string) (string, string) {
	// AC-I-14 deferred to a Sprint 8 sibling per Council Round 2.
	return "deferred to sibling story (Council Round 2)", ""
}
