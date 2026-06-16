package sequences

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// contactA is a fixed contact UUID for the preflight/bounce doubles (wsA/enrA
// live in idempotency_test.go).
var contactA = uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")

// preflightTx is a pgx.Tx double for Preflight / bounce.go. It answers each of
// the (few, distinct) QueryRow shapes by SQL-substring match and records every
// query issued and every Exec, so a test can both drive a verdict and prove
// which gates short-circuited. Embedding pgx.Tx (nil) makes any unstubbed call
// nil-deref — surfacing an unexpected DB touch as a failure.
type preflightTx struct {
	pgx.Tx

	// recipient lookup
	email        *string
	contactID    uuid.UUID
	noEnrollment bool

	// suppression check
	suppressedReason *string // non-nil → a row exists

	// volume cap
	capCount int

	// throttle
	lastSentAt *time.Time // non-nil → a recent send exists (throttled)

	// consecutive soft bounces (bounce.go)
	softRun int

	queries []string
	execs   []recordedExec
}

func (t *preflightTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	t.queries = append(t.queries, sql)
	switch {
	case strings.Contains(sql, "JOIN contacts c ON c.id = e.contact_id") && strings.Contains(sql, "WHERE e.id = $1"):
		if t.noEnrollment {
			return errRow{pgx.ErrNoRows}
		}
		return recipientRow{email: t.email, contactID: t.contactID}
	case strings.Contains(sql, "FROM email_suppression WHERE email"):
		if t.suppressedReason == nil {
			return errRow{pgx.ErrNoRows}
		}
		return scalarStrRow{s: *t.suppressedReason}
	case strings.Contains(sql, "count(*) FROM enrollment_steps"):
		return scalarIntRow{n: t.capCount}
	case strings.Contains(sql, "ORDER BY s.sent_at DESC") && strings.Contains(sql, "LIMIT 1"):
		if t.lastSentAt == nil {
			return errRow{pgx.ErrNoRows}
		}
		return timeRow{t: *t.lastSentAt}
	case strings.Contains(sql, "is_soft"):
		return scalarIntRow{n: t.softRun}
	default:
		return errRow{errors.New("preflightTx: unexpected QueryRow: " + sql)}
	}
}

func (t *preflightTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, recordedExec{sql: sql, args: args})
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (t *preflightTx) issued(substr string) bool {
	for _, q := range t.queries {
		if strings.Contains(q, substr) {
			return true
		}
	}
	return false
}

func (t *preflightTx) execMatching(substr string) (recordedExec, bool) {
	for _, e := range t.execs {
		if strings.Contains(e.sql, substr) {
			return e, true
		}
	}
	return recordedExec{}, false
}

// --- row scanners ---------------------------------------------------------

type recipientRow struct {
	email     *string
	contactID uuid.UUID
}

func (r recipientRow) Scan(dest ...any) error {
	if p, ok := dest[0].(**string); ok {
		*p = r.email
	}
	if p, ok := dest[1].(*uuid.UUID); ok {
		*p = r.contactID
	}
	return nil
}

type scalarStrRow struct{ s string }

func (r scalarStrRow) Scan(dest ...any) error {
	if p, ok := dest[0].(*string); ok {
		*p = r.s
	}
	return nil
}

type scalarIntRow struct{ n int }

func (r scalarIntRow) Scan(dest ...any) error {
	if p, ok := dest[0].(*int); ok {
		*p = r.n
	}
	return nil
}

type timeRow struct{ t time.Time }

func (r timeRow) Scan(dest ...any) error {
	if p, ok := dest[0].(*time.Time); ok {
		*p = r.t
	}
	return nil
}

func strptr(s string) *string { return &s }

// --- tests ----------------------------------------------------------------

// TestPreflight_Allow: a deliverable, un-suppressed, under-cap, un-throttled
// recipient clears every gate.
func TestPreflight_Allow(t *testing.T) {
	tx := &preflightTx{email: strptr("ok@example.fr"), contactID: contactA, capCount: 10}
	got, err := Preflight(context.Background(), tx, enrA, 0, DefaultLimits())
	if err != nil {
		t.Fatalf("Preflight error: %v", err)
	}
	if !got.Allowed() {
		t.Fatalf("verdict = %s, want allow", got.Verdict)
	}
	if got.Email != "ok@example.fr" {
		t.Errorf("Email = %q, want ok@example.fr", got.Email)
	}
}

// TestPreflight_Suppressed: a suppression row makes the verdict terminal AND
// short-circuits the deferrable gates — the cap and throttle queries are never
// issued, so a suppressed contact can never be "merely throttled" into a later
// send.
func TestPreflight_Suppressed(t *testing.T) {
	tx := &preflightTx{
		email:            strptr("bounced@example.fr"),
		contactID:        contactA,
		suppressedReason: strptr(SuppressionReasonHardBounce),
		// cap/throttle data present but must be ignored:
		capCount:   999999,
		lastSentAt: timeptr(time.Now()),
	}
	got, err := Preflight(context.Background(), tx, enrA, 2, DefaultLimits())
	if err != nil {
		t.Fatalf("Preflight error: %v", err)
	}
	if got.Verdict != PreflightSuppressed {
		t.Fatalf("verdict = %s, want suppressed", got.Verdict)
	}
	if got.Reason != SuppressionReasonHardBounce {
		t.Errorf("Reason = %q, want %q", got.Reason, SuppressionReasonHardBounce)
	}
	if tx.issued("count(*) FROM enrollment_steps") {
		t.Error("cap query issued after a suppression hit (should short-circuit)")
	}
	if tx.issued("ORDER BY s.sent_at DESC") {
		t.Error("throttle query issued after a suppression hit (should short-circuit)")
	}
}

// TestPreflight_SuppressedTransitionsToSuppressed: the documented contract — on
// a PreflightSuppressed verdict the caller drives Transition(→ StateSuppressed),
// which is a legal terminal edge from the enrolled state and never sends.
func TestPreflight_SuppressedTransitionsToSuppressed(t *testing.T) {
	pf := &preflightTx{email: strptr("x@example.fr"), contactID: contactA, suppressedReason: strptr(SuppressionReasonComplaint)}
	dec, err := Preflight(context.Background(), pf, enrA, 0, DefaultLimits())
	if err != nil || dec.Verdict != PreflightSuppressed {
		t.Fatalf("Preflight = (%+v, %v), want suppressed", dec, err)
	}

	// Caller applies the terminal transition on the same kind of workspace tx.
	tt := &transitionTx{fromState: string(StateEnrolled), wsID: wsA}
	if err := Transition(context.Background(), tt, enrA, StateSuppressed, dec.Reason); err != nil {
		t.Fatalf("Transition(→ suppressed) error: %v", err)
	}
	if !StateSuppressed.IsTerminal() {
		t.Error("StateSuppressed is not terminal")
	}
	if _, ok := tt.execMatching("UPDATE enrollments"); !ok {
		t.Error("suppression transition issued no UPDATE")
	}
}

// TestPreflight_Capped: at-or-over the monthly cap defers the send; RetryAfter
// is the start of next month.
func TestPreflight_Capped(t *testing.T) {
	tx := &preflightTx{email: strptr("c@example.fr"), contactID: contactA, capCount: 5000}
	got, err := Preflight(context.Background(), tx, enrA, 0, Limits{MonthlySendCap: 5000})
	if err != nil {
		t.Fatalf("Preflight error: %v", err)
	}
	if got.Verdict != PreflightCapped {
		t.Fatalf("verdict = %s, want capped", got.Verdict)
	}
	if !got.Deferrable() {
		t.Error("capped verdict should be Deferrable")
	}
	if got.RetryAfter.IsZero() || !got.RetryAfter.After(time.Now()) {
		t.Errorf("RetryAfter = %v, want a future month start", got.RetryAfter)
	}
	if got.RetryAfter.Day() != 1 {
		t.Errorf("RetryAfter day = %d, want 1 (start of month)", got.RetryAfter.Day())
	}
	// Throttle must not be consulted once capped.
	if tx.issued("ORDER BY s.sent_at DESC") {
		t.Error("throttle query issued after a cap hit (should short-circuit)")
	}
}

// TestPreflight_CapDisabled: MonthlySendCap <= 0 disables the cap entirely — the
// count query is never issued even with a huge backlog.
func TestPreflight_CapDisabled(t *testing.T) {
	tx := &preflightTx{email: strptr("u@example.fr"), contactID: contactA, capCount: 1_000_000}
	got, err := Preflight(context.Background(), tx, enrA, 0, Limits{MonthlySendCap: 0})
	if err != nil {
		t.Fatalf("Preflight error: %v", err)
	}
	if !got.Allowed() {
		t.Fatalf("verdict = %s, want allow (cap disabled)", got.Verdict)
	}
	if tx.issued("count(*) FROM enrollment_steps") {
		t.Error("cap count query issued although MonthlySendCap <= 0")
	}
}

// TestPreflight_Throttled: a step sent within the window defers the send;
// RetryAfter = last_sent_at + window.
func TestPreflight_Throttled(t *testing.T) {
	last := time.Now().Add(-2 * time.Hour)
	tx := &preflightTx{email: strptr("t@example.fr"), contactID: contactA, capCount: 1, lastSentAt: &last}
	got, err := Preflight(context.Background(), tx, enrA, 1, DefaultLimits())
	if err != nil {
		t.Fatalf("Preflight error: %v", err)
	}
	if got.Verdict != PreflightThrottled {
		t.Fatalf("verdict = %s, want throttled", got.Verdict)
	}
	want := last.Add(DefaultThrottleWindow)
	if !got.RetryAfter.Equal(want) {
		t.Errorf("RetryAfter = %v, want %v (last_sent_at + window)", got.RetryAfter, want)
	}
}

// TestPreflight_NoRecipientEmail: an enrollment whose contact has no email is
// undeliverable.
func TestPreflight_NoRecipientEmail(t *testing.T) {
	tx := &preflightTx{email: nil, contactID: contactA}
	_, err := Preflight(context.Background(), tx, enrA, 0, DefaultLimits())
	if !errors.Is(err, ErrNoRecipientEmail) {
		t.Fatalf("error = %v, want ErrNoRecipientEmail", err)
	}
	if tx.issued("FROM email_suppression") {
		t.Error("suppression query issued for an address-less contact")
	}
}

// TestPreflight_EnrollmentNotFound: a missing enrollment row maps to
// ErrEnrollmentNotFound with no further queries.
func TestPreflight_EnrollmentNotFound(t *testing.T) {
	tx := &preflightTx{noEnrollment: true}
	_, err := Preflight(context.Background(), tx, enrA, 0, DefaultLimits())
	if !errors.Is(err, ErrEnrollmentNotFound) {
		t.Fatalf("error = %v, want ErrEnrollmentNotFound", err)
	}
}

func timeptr(t time.Time) *time.Time { return &t }
