//go:build integration

// Integration coverage for migration 0025_sequences_schema_actor_type.sql.
// Provisions a fresh workspace and asserts the v1 native-sequences durable
// state foundation (ADR-004 rev 2 §1) lands intact:
//
//   - enrollments + enrollment_steps tables exist;
//   - enrollment_state / step_send_state enums exist with the exact ADR label
//     sets, in order (the sqlc-generated Go constants depend on the order);
//   - the Brandur-style partial unique index uniq_enrollment_step_active both
//     EXISTS and ENFORCES at-most-once on active rows while allowing a
//     superseded attempt to coexist with a fresh one;
//   - core.audit_log.actor_type is NOT NULL DEFAULT 'human_api'
//     (ADR-004 rev 2 §Q6 / TO RESOLVE-S1).
//
// Reuses migrationPaths / connectWithRetry from provision_test.go.

package domain_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestProvision_SequencesSchema(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(migrationPaths(t)...),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("terminate: %v", err)
		}
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	conn := connectWithRetry(ctx, t, connStr, 15*time.Second)
	defer func() { _ = conn.Close(ctx) }()

	wsID := uuid.New()
	var roleName string
	if err := conn.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&roleName); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}

	// --- tables exist ---
	for _, table := range []string{"enrollments", "enrollment_steps"} {
		var exists bool
		if err := conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = $1 AND table_name = $2
			)`, roleName, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s.%s missing after provisioning", roleName, table)
		}
	}

	// --- enum types exist in the workspace schema, with the exact ADR labels
	//     and order (the generated Go enum constants depend on the order) ---
	assertEnumLabels(ctx, t, conn, roleName, "enrollment_state", []string{
		"enrolled", "step_sent", "waiting_reply", "reply_received", "ooo_detected",
		"failed", "bounced", "unsubscribed", "suppressed", "completed",
	})
	assertEnumLabels(ctx, t, conn, roleName, "step_send_state", []string{
		"pending", "sent", "delivered", "bounced", "cancelled", "superseded",
	})

	// --- enrollment_steps carries the ADR-mandated correlation columns ---
	wantCols := []string{"brevo_message_id", "rfc_message_id", "idempotency_key", "step_index", "state", "scheduled_for"}
	gotCols := map[string]bool{}
	rows, err := conn.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'enrollment_steps'`, roleName)
	if err != nil {
		t.Fatalf("query enrollment_steps columns: %v", err)
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		gotCols[c] = true
	}
	rows.Close()
	for _, c := range wantCols {
		if !gotCols[c] {
			t.Errorf("enrollment_steps missing column %q", c)
		}
	}

	// --- indexes exist ---
	assertIndex(ctx, t, conn, roleName, "enrollments", "idx_enr_state_next")
	assertIndex(ctx, t, conn, roleName, "enrollment_steps", "uniq_enrollment_step_active")
	assertIndex(ctx, t, conn, roleName, "enrollment_steps", "idx_step_brevo_msgid")
	assertIndex(ctx, t, conn, roleName, "enrollment_steps", "idx_step_rfc_msgid")

	// --- SEMANTIC: the partial unique index is the durable at-most-once
	//     backstop. Pin the workspace schema and exercise it directly. ---
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s, core, public", roleName)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	enrID := uuid.New()
	if _, err := conn.Exec(ctx,
		`INSERT INTO enrollments (id, sequence_id, contact_id, workspace_id)
		 VALUES ($1, $2, $3, $4)`,
		enrID, uuid.New(), uuid.New(), wsID); err != nil {
		t.Fatalf("insert enrollment: %v", err)
	}

	// state values below are test-controlled literals — no injection surface.
	insStep := func(state string) error {
		_, err := conn.Exec(ctx,
			`INSERT INTO enrollment_steps (enrollment_id, step_index, state, scheduled_for, idempotency_key)
			 VALUES ($1, 0, '`+state+`'::step_send_state, now(), $2)`,
			enrID, uuid.NewString())
		return err
	}

	// First active (pending) row: OK.
	if err := insStep("pending"); err != nil {
		t.Fatalf("first active step insert: %v", err)
	}
	// Second active row for the same (enrollment_id, step_index) MUST collide.
	if err := insStep("sent"); err == nil {
		t.Errorf("second active step insert succeeded — uniq_enrollment_step_active did not enforce at-most-once")
	}
	// Supersede the first attempt; a fresh active row must then be allowed
	// (the Brandur retry pattern: superseded rows fall outside the index).
	if _, err := conn.Exec(ctx,
		`UPDATE enrollment_steps SET state = 'superseded'
		 WHERE enrollment_id = $1 AND step_index = 0`, enrID); err != nil {
		t.Fatalf("supersede step: %v", err)
	}
	if err := insStep("pending"); err != nil {
		t.Errorf("post-supersede active insert failed — index over-constrains: %v", err)
	}

	// --- audit_log.actor_type is NOT NULL DEFAULT 'human_api' (Q6 / S1) ---
	var isNullable, colDefault string
	if err := conn.QueryRow(ctx, `
		SELECT is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = 'core' AND table_name = 'audit_log' AND column_name = 'actor_type'`).
		Scan(&isNullable, &colDefault); err != nil {
		t.Fatalf("check audit_log.actor_type: %v", err)
	}
	if isNullable != "NO" {
		t.Errorf("audit_log.actor_type is_nullable = %q, want NO", isNullable)
	}
	if !strings.Contains(colDefault, "human_api") {
		t.Errorf("audit_log.actor_type default = %q, want it to contain 'human_api'", colDefault)
	}

	// SEMANTIC: an INSERT that omits actor_type now attributes to 'human_api'
	// via the column default rather than landing a NULL. workspace_id is
	// omitted (nullable) — the provision function does not insert a
	// core.workspaces row, so a real wsID would trip the FK.
	var gotActor string
	if err := conn.QueryRow(ctx, `
		INSERT INTO core.audit_log (event, payload)
		VALUES ('test.default_actor', '{}'::jsonb)
		RETURNING actor_type`).Scan(&gotActor); err != nil {
		t.Fatalf("insert audit row without actor_type: %v", err)
	}
	if gotActor != "human_api" {
		t.Errorf("omitted actor_type defaulted to %q, want human_api", gotActor)
	}
}

// assertEnumLabels verifies the enum type schema.enum has exactly want, in order.
func assertEnumLabels(ctx context.Context, t *testing.T, conn *pgx.Conn, schema, enum string, want []string) {
	t.Helper()
	rows, err := conn.Query(ctx, `
		SELECT e.enumlabel
		FROM pg_enum e
		JOIN pg_type t ON e.enumtypid = t.oid
		JOIN pg_namespace n ON t.typnamespace = n.oid
		WHERE n.nspname = $1 AND t.typname = $2
		ORDER BY e.enumsortorder`, schema, enum)
	if err != nil {
		t.Fatalf("query enum %s labels: %v", enum, err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			t.Fatalf("scan enum label: %v", err)
		}
		got = append(got, l)
	}
	if len(got) == 0 {
		t.Errorf("enum %s.%s missing or empty after provisioning", schema, enum)
		return
	}
	if len(got) != len(want) {
		t.Errorf("enum %s.%s labels = %v, want %v", schema, enum, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("enum %s.%s label[%d] = %q, want %q", schema, enum, i, got[i], want[i])
		}
	}
}

func assertIndex(ctx context.Context, t *testing.T, conn *pgx.Conn, schema, table, index string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = $1 AND tablename = $2 AND indexname = $3
		)`, schema, table, index).Scan(&exists); err != nil {
		t.Fatalf("check index %s: %v", index, err)
	}
	if !exists {
		t.Errorf("index %s on %s.%s does not exist", index, schema, table)
	}
}
