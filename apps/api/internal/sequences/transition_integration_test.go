//go:build integration

// Integration coverage for sequences.Transition + migration
// 0026_sequence_audit_emit_fn.sql, exercised against a real workspace-role
// connection (the production worker scenario: db.TenantPool.AcquireTx logs in as
// workspace_<hex>, simple query protocol, search_path = the workspace schema +
// public, NO access to schema core).
//
// It proves the four properties the unit tests cannot, because they hinge on
// real Postgres privileges and transaction semantics:
//
//   - a workspace-role tx CAN audit its own state change through the
//     SECURITY DEFINER function core.lecrm_emit_audit, even though it cannot
//     touch core.audit_log directly (the whole reason 0026 exists);
//   - the state change + audit row are atomic — rolling the tx back discards
//     BOTH (ADR-009 §7.2 fail-closed);
//   - the prod (InvalidAudit) path lands a sequences.transition.invalid row for
//     an illegal transition and returns an error (the "500 + audit" contract);
//   - the session_user guard stops tenant A forging an audit row for tenant B.
//
// Runs only under -tags=integration and is skipped when Docker is unreachable.
package sequences

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// transitionITEnv is the shared fixture: a provisioned workspace, a superuser
// connection (for assertions/seeding), and a connection authenticated AS the
// workspace role (the real worker identity).
type transitionITEnv struct {
	wsID     uuid.UUID
	roleName string
	su       *pgx.Conn // superuser
	role     *pgx.Conn // logged in as workspace_<hex>, simple protocol
}

func TestTransition_Integration(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(seqMigrationPaths(t)...),
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
	su := connectWithRetrySeq(ctx, t, connStr, 15*time.Second)
	t.Cleanup(func() { _ = su.Close(ctx) })

	// Provision a workspace and register it in core.workspaces (the audit_log FK
	// references it; without the row the SDF's INSERT fails on workspace_id).
	wsID := uuid.New()
	var roleName string
	if err := su.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&roleName); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}
	if _, err := su.Exec(ctx,
		`INSERT INTO core.workspaces (id, slug, role_name) VALUES ($1, $2, $3)`,
		wsID, "seq-it", roleName); err != nil {
		t.Fatalf("register workspace: %v", err)
	}

	// Give the workspace role a known password so we can log in AS it — the
	// faithful reproduction of AcquireTx (session_user = workspace_<hex>).
	const rolePw = "it-role-pw"
	if _, err := su.Exec(ctx, fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", pgx.Identifier{roleName}.Sanitize(), rolePw)); err != nil {
		t.Fatalf("set role password: %v", err)
	}

	roleConn := connectAsRole(ctx, t, connStr, roleName, rolePw)
	t.Cleanup(func() { _ = roleConn.Close(ctx) })

	env := &transitionITEnv{wsID: wsID, roleName: roleName, su: su, role: roleConn}

	// session_user MUST be the workspace role for the guard tests to be meaningful.
	var sessionUser string
	if err := roleConn.QueryRow(ctx, "SELECT session_user").Scan(&sessionUser); err != nil {
		t.Fatalf("session_user: %v", err)
	}
	if sessionUser != roleName {
		t.Fatalf("role connection session_user = %q, want %q", sessionUser, roleName)
	}

	t.Run("HappyPath_StateAndAuditCommitTogether", env.testHappyPath)
	t.Run("RollbackAtomicity", env.testRollbackAtomicity)
	t.Run("ProdInvalidEmitsAuditAndReturns", env.testProdInvalidAudit)
	t.Run("CrossTenantAuditForgeryRejected", env.testCrossTenantGuard)
}

// seedEnrollment inserts a fresh enrolled row via the role connection (the role
// owns its schema) and returns its id.
func (e *transitionITEnv) seedEnrollment(ctx context.Context, t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.role.Exec(ctx,
		`INSERT INTO enrollments (id, sequence_id, contact_id, workspace_id)
		 VALUES ($1, $2, $3, $4)`,
		id, uuid.New(), uuid.New(), e.wsID); err != nil {
		t.Fatalf("seed enrollment: %v", err)
	}
	return id
}

func (e *transitionITEnv) countAudit(ctx context.Context, t *testing.T, enrollmentID uuid.UUID, event string) int {
	t.Helper()
	var n int
	if err := e.su.QueryRow(ctx,
		`SELECT count(*) FROM core.audit_log
		  WHERE event = $1 AND workspace_id = $2 AND payload->>'enrollment_id' = $3`,
		event, e.wsID, enrollmentID.String()).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	return n
}

func (e *transitionITEnv) state(ctx context.Context, t *testing.T, enrollmentID uuid.UUID) string {
	t.Helper()
	var s string
	if err := e.role.QueryRow(ctx, `SELECT state::text FROM enrollments WHERE id = $1`, enrollmentID).Scan(&s); err != nil {
		t.Fatalf("read state: %v", err)
	}
	return s
}

// testHappyPath: a workspace-role tx transitions enrolled→step_sent and commits.
// Both the state change and the sequences.step_sent audit row (written through
// the SDF, attributed internal_service) must be visible afterwards.
func (e *transitionITEnv) testHappyPath(t *testing.T) {
	ctx := context.Background()
	enrID := e.seedEnrollment(ctx, t)

	tx, err := e.role.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := Transition(ctx, tx, enrID, StateStepSent, "first send"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Transition: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if got := e.state(ctx, t, enrID); got != string(StateStepSent) {
		t.Errorf("state after commit = %q, want step_sent", got)
	}
	if n := e.countAudit(ctx, t, enrID, AuditEventStepSent); n != 1 {
		t.Errorf("sequences.step_sent audit rows = %d, want 1", n)
	}

	// The audit row carries the internal_service attribution.
	var actor string
	if err := e.su.QueryRow(ctx,
		`SELECT actor_type FROM core.audit_log
		  WHERE event = $1 AND payload->>'enrollment_id' = $2`,
		AuditEventStepSent, enrID.String()).Scan(&actor); err != nil {
		t.Fatalf("read actor_type: %v", err)
	}
	if actor != "internal_service" {
		t.Errorf("audit actor_type = %q, want internal_service", actor)
	}
}

// testRollbackAtomicity: a Transition whose tx is rolled back must leave NEITHER
// the state change NOR the audit row — they are bound to the same transaction
// (ADR-009 §7.2). This is the atomicity that makes "step sent but audit lost"
// impossible.
func (e *transitionITEnv) testRollbackAtomicity(t *testing.T) {
	ctx := context.Background()
	enrID := e.seedEnrollment(ctx, t)

	tx, err := e.role.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := Transition(ctx, tx, enrID, StateStepSent, "will be rolled back"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Transition: %v", err)
	}
	// Simulate the worker aborting after the transition (e.g. a later step fails).
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if got := e.state(ctx, t, enrID); got != string(StateEnrolled) {
		t.Errorf("state after rollback = %q, want enrolled (unchanged)", got)
	}
	if n := e.countAudit(ctx, t, enrID, AuditEventStepSent); n != 0 {
		t.Errorf("audit rows after rollback = %d, want 0 (audit must roll back with the state)", n)
	}
}

// testProdInvalidAudit: in InvalidAudit mode an illegal transition emits
// sequences.transition.invalid (committed) and returns ErrInvalidTransition —
// the "return 500 + audit_log sequences.transition.invalid in prod" contract
// (ADR-004 rev 2 §2). The state is untouched.
func (e *transitionITEnv) testProdInvalidAudit(t *testing.T) {
	ctx := context.Background()
	enrID := e.seedEnrollment(ctx, t)

	tx, err := e.role.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// enrolled → reply_received is illegal.
	terr := Transition(ctx, tx, enrID, StateReplyReceived, "bug",
		WithInvalidMode(InvalidAudit), WithCaller("integration"))
	if !errors.Is(terr, ErrInvalidTransition) {
		_ = tx.Rollback(ctx)
		t.Fatalf("Transition error = %v, want ErrInvalidTransition", terr)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit invalid-audit tx: %v", err)
	}

	if got := e.state(ctx, t, enrID); got != string(StateEnrolled) {
		t.Errorf("state after invalid transition = %q, want enrolled (unchanged)", got)
	}
	if n := e.countAudit(ctx, t, enrID, AuditEventTransitionInvalid); n != 1 {
		t.Errorf("sequences.transition.invalid rows = %d, want 1", n)
	}
	// The trace carries the attempted target and caller.
	var toAttempted, caller string
	if err := e.su.QueryRow(ctx,
		`SELECT payload->>'to_attempted', payload->>'caller' FROM core.audit_log
		  WHERE event = $1 AND payload->>'enrollment_id' = $2`,
		AuditEventTransitionInvalid, enrID.String()).Scan(&toAttempted, &caller); err != nil {
		t.Fatalf("read invalid-audit payload: %v", err)
	}
	if toAttempted != string(StateReplyReceived) || caller != "integration" {
		t.Errorf("invalid-audit payload = (to_attempted=%q, caller=%q), want (reply_received, integration)", toAttempted, caller)
	}
}

// testCrossTenantGuard: the SDF's session_user guard must reject a workspace-A
// connection trying to emit an audit row attributed to a different workspace_id
// — the tenant-isolation crux of 0026. Emitting for its OWN workspace succeeds.
func (e *transitionITEnv) testCrossTenantGuard(t *testing.T) {
	ctx := context.Background()
	otherWS := uuid.New()

	// Forging for another workspace → insufficient_privilege.
	_, err := e.role.Exec(ctx,
		`SELECT core.lecrm_emit_audit($1, $2, $3, $4, $5)`,
		"sequences.step_sent", otherWS, "internal_service", uuid.NullUUID{}, "{}")
	if err == nil {
		t.Fatal("cross-tenant audit forgery succeeded — session_user guard did not fire")
	}
	var pgErr *pgconnError
	if !asPgError(err, &pgErr) || pgErr.code != "42501" { // insufficient_privilege
		t.Errorf("cross-tenant emit error = %v, want SQLSTATE 42501 (insufficient_privilege)", err)
	}

	// Emitting for its OWN workspace is allowed.
	if _, err := e.role.Exec(ctx,
		`SELECT core.lecrm_emit_audit($1, $2, $3, $4, $5)`,
		"sequences.step_sent", e.wsID, "internal_service", uuid.NullUUID{}, "{}"); err != nil {
		t.Errorf("own-workspace audit emit rejected: %v", err)
	}
}

// --- test plumbing ---

// pgconnError is a minimal view over *pgconn.PgError so this file does not need
// to import pgconn just for the SQLSTATE.
type pgconnError struct{ code string }

func asPgError(err error, out **pgconnError) bool {
	type sqlStater interface{ SQLState() string }
	var s sqlStater
	if errors.As(err, &s) {
		*out = &pgconnError{code: s.SQLState()}
		return true
	}
	return false
}

func connectAsRole(ctx context.Context, t *testing.T, superuserConnStr, role, pw string) *pgx.Conn {
	t.Helper()
	cfg, err := pgx.ParseConfig(superuserConnStr)
	if err != nil {
		t.Fatalf("parse conn config: %v", err)
	}
	cfg.User = role
	cfg.Password = pw
	// Mirror the production tenant pool, which runs the simple query protocol.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect as role %s: %v", role, err)
	}
	return conn
}

func connectWithRetrySeq(ctx context.Context, t *testing.T, connStr string, maxWait time.Duration) *pgx.Conn {
	t.Helper()
	deadline := time.Now().Add(maxWait)
	for {
		conn, err := pgx.Connect(ctx, connStr)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect after %s: %v", maxWait, err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("connect: %v", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// seqMigrationPaths returns the full production migration chain, sorted, so the
// provisioned workspace gets the real prod schema including 0026. Mirrors the
// domain/tenantpair harness (glob keeps it in lockstep with prod).
func seqMigrationPaths(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is apps/api/internal/sequences/transition_integration_test.go;
	// four levels up reaches the repo root (leCRM/).
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	migrationsDir := filepath.Join(repoRoot, "packages", "db", "migrations")
	paths, err := filepath.Glob(filepath.Join(migrationsDir, "[0-9]*.sql"))
	if err != nil {
		t.Fatalf("glob migrations in %s: %v", migrationsDir, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no migrations found in %s", migrationsDir)
	}
	sort.Strings(paths)
	return paths
}
