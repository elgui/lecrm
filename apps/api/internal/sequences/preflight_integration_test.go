//go:build integration

// Integration coverage for sequences.Preflight + the bounce policy (bounce.go),
// run against a REAL workspace-role connection on the production schema (the
// send_step-worker scenario: db.TenantPool.AcquireTx logs in as workspace_<hex>,
// simple query protocol, search_path = the workspace schema + public).
//
// The unit tests drive Preflight/ApplyBounce through a SQL-substring-matched
// pgx.Tx double, so they prove the decision logic but NOT that the SQL is valid
// against the provisioned schema. This file closes that gap: every query
// (recipient resolve, suppression probe, monthly-cap count, per-recipient
// throttle, consecutive-soft-bounce window, suppression upsert) executes for
// real, so a column/typo/enum mistake fails here.
//
// It shares the connect/migration plumbing with transition_integration_test.go
// (connectAsRole, connectWithRetrySeq, seqMigrationPaths). Runs only under
// -tags=integration and is skipped when Docker is unreachable.
package sequences

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type preflightITEnv struct {
	wsID uuid.UUID
	su   *pgx.Conn // superuser
	role *pgx.Conn // logged in as workspace_<hex>, simple protocol
}

func TestPreflight_Integration(t *testing.T) {
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

	wsID := uuid.New()
	var roleName string
	if err := su.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&roleName); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}
	if _, err := su.Exec(ctx,
		`INSERT INTO core.workspaces (id, slug, role_name) VALUES ($1, $2, $3)`,
		wsID, "preflight-it", roleName); err != nil {
		t.Fatalf("register workspace: %v", err)
	}
	const rolePw = "it-role-pw"
	if _, err := su.Exec(ctx, fmt.Sprintf("ALTER ROLE %s WITH PASSWORD '%s'", pgx.Identifier{roleName}.Sanitize(), rolePw)); err != nil {
		t.Fatalf("set role password: %v", err)
	}
	roleConn := connectAsRole(ctx, t, connStr, roleName, rolePw)
	t.Cleanup(func() { _ = roleConn.Close(ctx) })

	env := &preflightITEnv{wsID: wsID, su: su, role: roleConn}

	t.Run("Allow_DeliverableUntouchedRecipient", env.testAllow)
	t.Run("Suppressed_NeverReSends", env.testSuppressed)
	t.Run("Throttled_RecentSendToContact", env.testThrottled)
	t.Run("Capped_MonthlyCapReached", env.testCapped)
	t.Run("NoRecipientEmail", env.testNoEmail)
	t.Run("SoftBounceThreeConsecutive_Suppresses", env.testSoftBounceSuppression)
}

// --- seeding helpers (role owns its schema) -------------------------------

func (e *preflightITEnv) seedContact(ctx context.Context, t *testing.T, email *string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.role.Exec(ctx,
		`INSERT INTO contacts (id, first_name, last_name, email) VALUES ($1, 'Test', 'Contact', $2)`,
		id, email); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return id
}

func (e *preflightITEnv) seedEnrollment(ctx context.Context, t *testing.T, contactID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.role.Exec(ctx,
		`INSERT INTO enrollments (id, sequence_id, contact_id, workspace_id) VALUES ($1, $2, $3, $4)`,
		id, uuid.New(), contactID, e.wsID); err != nil {
		t.Fatalf("seed enrollment: %v", err)
	}
	return id
}

// seedSentStep inserts an enrollment_steps row that was actually sent at sentAt.
// bounceType "" → a plain send (state 'sent'); "soft"/"hard" → a bounced send
// (state 'bounced', bounced_at = sentAt).
func (e *preflightITEnv) seedSentStep(ctx context.Context, t *testing.T, enrollmentID uuid.UUID, stepIndex int, sentAt time.Time, bounceType string) {
	t.Helper()
	state := "sent"
	var bouncedAt *time.Time
	var bt *string
	if bounceType != "" {
		state = "bounced"
		bouncedAt = &sentAt
		bt = &bounceType
	}
	if _, err := e.role.Exec(ctx,
		`INSERT INTO enrollment_steps
		   (id, enrollment_id, step_index, state, scheduled_for, sent_at, bounced_at, bounce_type, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8)`,
		uuid.New(), enrollmentID, stepIndex, state, sentAt, bouncedAt, bt, uuid.NewString()); err != nil {
		t.Fatalf("seed sent step: %v", err)
	}
}

func (e *preflightITEnv) suppressionExists(ctx context.Context, t *testing.T, email string) bool {
	t.Helper()
	var n int
	if err := e.role.QueryRow(ctx, `SELECT count(*) FROM email_suppression WHERE email = $1`, email).Scan(&n); err != nil {
		t.Fatalf("count suppression: %v", err)
	}
	return n > 0
}

func (e *preflightITEnv) monthlyStepCount(ctx context.Context, t *testing.T) int {
	t.Helper()
	var n int
	if err := e.role.QueryRow(ctx,
		`SELECT count(*) FROM enrollment_steps WHERE sent_at >= date_trunc('month', now())`).Scan(&n); err != nil {
		t.Fatalf("count monthly steps: %v", err)
	}
	return n
}

// run is a tiny helper that runs Preflight inside a real role tx and returns the
// decision (mirroring how the send_step worker shares the worker tx).
func (e *preflightITEnv) preflight(ctx context.Context, t *testing.T, enrollmentID uuid.UUID, limits Limits) PreflightDecision {
	t.Helper()
	tx, err := e.role.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	dec, err := Preflight(ctx, tx, enrollmentID, 0, limits)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	return dec
}

// --- subtests -------------------------------------------------------------

// capOff disables the workspace-wide cap so subtests that don't exercise it are
// not perturbed by steps other subtests seeded.
var capOff = Limits{MonthlySendCap: 0}

func (e *preflightITEnv) testAllow(t *testing.T) {
	ctx := context.Background()
	email := "ok@example.fr"
	c := e.seedContact(ctx, t, &email)
	enr := e.seedEnrollment(ctx, t, c)

	dec := e.preflight(ctx, t, enr, capOff)
	if !dec.Allowed() {
		t.Fatalf("verdict = %s, want allow", dec.Verdict)
	}
	if dec.Email != email {
		t.Errorf("Email = %q, want %q", dec.Email, email)
	}
}

func (e *preflightITEnv) testSuppressed(t *testing.T) {
	ctx := context.Background()
	email := "suppressed@example.fr"
	c := e.seedContact(ctx, t, &email)
	enr := e.seedEnrollment(ctx, t, c)
	if _, err := e.role.Exec(ctx,
		`INSERT INTO email_suppression (email, reason) VALUES ($1, $2)`, email, SuppressionReasonComplaint); err != nil {
		t.Fatalf("seed suppression: %v", err)
	}

	dec := e.preflight(ctx, t, enr, capOff)
	if dec.Verdict != PreflightSuppressed {
		t.Fatalf("verdict = %s, want suppressed", dec.Verdict)
	}
	if dec.Reason != SuppressionReasonComplaint {
		t.Errorf("Reason = %q, want %q", dec.Reason, SuppressionReasonComplaint)
	}
}

func (e *preflightITEnv) testThrottled(t *testing.T) {
	ctx := context.Background()
	email := "throttled@example.fr"
	c := e.seedContact(ctx, t, &email)
	enr := e.seedEnrollment(ctx, t, c)
	// A step sent to this contact 2h ago is inside the default 24h window.
	e.seedSentStep(ctx, t, enr, 0, time.Now().Add(-2*time.Hour), "")

	dec := e.preflight(ctx, t, enr, capOff)
	if dec.Verdict != PreflightThrottled {
		t.Fatalf("verdict = %s, want throttled", dec.Verdict)
	}
	if dec.RetryAfter.IsZero() {
		t.Error("throttled decision has a zero RetryAfter")
	}
}

func (e *preflightITEnv) testCapped(t *testing.T) {
	ctx := context.Background()
	email := "capped@example.fr"
	c := e.seedContact(ctx, t, &email)
	enr := e.seedEnrollment(ctx, t, c)

	// Set the cap to the count already sent this month, so the workspace is
	// exactly at its limit regardless of what other subtests seeded.
	monthCap := e.monthlyStepCount(ctx, t)
	if monthCap == 0 {
		// Ensure at least one send exists so the cap is meaningfully "reached".
		e.seedSentStep(ctx, t, enr, 0, time.Now().Add(-48*time.Hour), "") // >24h ago → not throttled
		monthCap = e.monthlyStepCount(ctx, t)
	}

	dec := e.preflight(ctx, t, enr, Limits{MonthlySendCap: monthCap})
	if dec.Verdict != PreflightCapped {
		t.Fatalf("verdict = %s, want capped (cap=%d)", dec.Verdict, monthCap)
	}
	if dec.RetryAfter.Day() != 1 {
		t.Errorf("RetryAfter day = %d, want 1 (start of next month)", dec.RetryAfter.Day())
	}
}

func (e *preflightITEnv) testNoEmail(t *testing.T) {
	ctx := context.Background()
	c := e.seedContact(ctx, t, nil) // NULL email
	enr := e.seedEnrollment(ctx, t, c)

	tx, err := e.role.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := Preflight(ctx, tx, enr, 0, capOff); !errors.Is(err, ErrNoRecipientEmail) {
		t.Fatalf("error = %v, want ErrNoRecipientEmail", err)
	}
}

// testSoftBounceSuppression: three consecutive soft bounces to one address make
// ConsecutiveSoftBounces report 3; ApplyBounce then writes a suppression row,
// and a subsequent Preflight on a fresh enrollment to that address is suppressed
// — the contact never re-sends (ADR-004 rev 2 §8 bounce policy).
func (e *preflightITEnv) testSoftBounceSuppression(t *testing.T) {
	ctx := context.Background()
	email := "softbounce@example.fr"
	c := e.seedContact(ctx, t, &email)
	enr := e.seedEnrollment(ctx, t, c)

	base := time.Now().Add(-72 * time.Hour)
	for i := 0; i < 3; i++ {
		e.seedSentStep(ctx, t, enr, i, base.Add(time.Duration(i)*time.Hour), "soft")
	}

	tx, err := e.role.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	n, err := ConsecutiveSoftBounces(ctx, tx, email)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ConsecutiveSoftBounces: %v", err)
	}
	if n != 3 {
		_ = tx.Rollback(ctx)
		t.Fatalf("consecutive soft bounces = %d, want 3", n)
	}
	action, err := ApplyBounce(ctx, tx, email, BounceSoft, DefaultLimits())
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("ApplyBounce: %v", err)
	}
	if !action.Suppress || action.Reason != SuppressionReasonHardBounce {
		_ = tx.Rollback(ctx)
		t.Fatalf("action = %+v, want suppress/hard_bounce", action)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit ApplyBounce: %v", err)
	}

	if !e.suppressionExists(ctx, t, email) {
		t.Fatal("ApplyBounce did not persist the suppression row")
	}

	// A new enrollment to the now-suppressed address is blocked pre-send.
	enr2 := e.seedEnrollment(ctx, t, c)
	dec := e.preflight(ctx, t, enr2, capOff)
	if dec.Verdict != PreflightSuppressed {
		t.Fatalf("verdict = %s, want suppressed after soft-bounce threshold", dec.Verdict)
	}
}
