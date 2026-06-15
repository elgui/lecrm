package sequences

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Preflight enforces the ADR-004 rev 2 §8 deliverability discipline that gates
// every sequence send. It is called inside the sequences.send_step job, on the
// SAME workspace-scoped transaction the worker already opened (db.TenantPool.
// AcquireTx), BEFORE any Brevo API call — a bad send damages the shared-account
// reputation that is the deliverability moat, so the cheap DB checks run first.
//
// Three gates, in priority order (§8, carried over from rev 1 §5/§6):
//
//  1. Suppression — the recipient's address is in the per-workspace
//     email_suppression table (ADR-003 §Mitigations item 3). This is terminal:
//     the caller transitions the enrollment to StateSuppressed and never sends
//     to this address again. Checked FIRST because it outranks the deferrable
//     gates — a suppressed recipient is done, not merely delayed.
//  2. Volume cap — the workspace has already sent `MonthlySendCap` steps this
//     calendar month. Deferrable: the caller reschedules send_step for the start
//     of next month (PreflightDecision.RetryAfter).
//  3. Throttle — a step was sent to this contact_id within ThrottleWindow (≤1
//     step / 24h per recipient, regardless of sequence). Deferrable: reschedule
//     for last_sent_at + window.
//
// Preflight performs NO writes and never transitions the enrollment itself — it
// returns a PreflightDecision and lets the send_step handler orchestrate the
// transition / reschedule, keeping Transition the single write path for
// enrollment state (§2). Table names are unqualified so they resolve through the
// workspace role's search_path (= workspace_<hex>, public), exactly as
// transition.go and the tenant pool's simple query protocol expect.
//
// The contact's email may be NULL (contacts.email is nullable); an enrollment to
// an address-less contact cannot be sent and returns ErrNoRecipientEmail, which
// the caller maps to StateFailed. A missing enrollment returns
// ErrEnrollmentNotFound (shared with Transition).
func Preflight(
	ctx context.Context,
	tx pgx.Tx,
	enrollmentID uuid.UUID,
	stepIndex int,
	limits Limits,
) (PreflightDecision, error) {
	limits = limits.withDefaults()

	// Resolve the recipient. enrollments → contacts on contact_id; email is
	// nullable, scanned into *string so a NULL is distinguishable from "".
	var (
		email     *string
		contactID uuid.UUID
	)
	err := tx.QueryRow(ctx,
		`SELECT c.email, e.contact_id
		   FROM enrollments e
		   JOIN contacts c ON c.id = e.contact_id
		  WHERE e.id = $1`,
		enrollmentID,
	).Scan(&email, &contactID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PreflightDecision{}, fmt.Errorf("%w: %s", ErrEnrollmentNotFound, enrollmentID)
	}
	if err != nil {
		return PreflightDecision{}, fmt.Errorf("sequences: preflight resolve recipient %s: %w", enrollmentID, err)
	}
	if email == nil || *email == "" {
		return PreflightDecision{}, fmt.Errorf("%w: enrollment %s", ErrNoRecipientEmail, enrollmentID)
	}

	// 1. Suppression — terminal. A row in email_suppression means a prior bounce,
	//    complaint, or unsubscribe (or the soft-bounce threshold, see bounce.go)
	//    permanently disqualified this address.
	var suppReason string
	err = tx.QueryRow(ctx,
		`SELECT reason FROM email_suppression WHERE email = $1`,
		*email,
	).Scan(&suppReason)
	switch {
	case err == nil:
		return PreflightDecision{
			Verdict:   PreflightSuppressed,
			StepIndex: stepIndex,
			Email:     *email,
			Reason:    suppReason,
		}, nil
	case errors.Is(err, pgx.ErrNoRows):
		// not suppressed — continue
	default:
		return PreflightDecision{}, fmt.Errorf("sequences: preflight suppression check %s: %w", enrollmentID, err)
	}

	// 2. Volume cap — workspace-wide, deferrable. Count steps actually sent this
	//    calendar month (sent_at set; pending/cancelled/superseded rows have a
	//    NULL sent_at and are excluded). Disabled when MonthlySendCap <= 0.
	if limits.MonthlySendCap > 0 {
		var sentThisMonth int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM enrollment_steps
			  WHERE sent_at >= date_trunc('month', now())`,
		).Scan(&sentThisMonth); err != nil {
			return PreflightDecision{}, fmt.Errorf("sequences: preflight cap count %s: %w", enrollmentID, err)
		}
		if sentThisMonth >= limits.MonthlySendCap {
			return PreflightDecision{
				Verdict:    PreflightCapped,
				StepIndex:  stepIndex,
				Email:      *email,
				Reason:     fmt.Sprintf("monthly_send_cap reached: %d/%d sent this month", sentThisMonth, limits.MonthlySendCap),
				RetryAfter: startOfNextMonth(time.Now()),
			}, nil
		}
	}

	// 3. Throttle — per-recipient, deferrable. The most recent step sent to this
	//    contact within the window (across ALL sequences) blocks a second send.
	//    The window cutoff is computed here and compared in SQL so the check is a
	//    single index probe; the returned sent_at gives the caller a precise
	//    reschedule time.
	cutoff := time.Now().Add(-limits.ThrottleWindow)
	var lastSentAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT s.sent_at
		   FROM enrollment_steps s
		   JOIN enrollments e ON e.id = s.enrollment_id
		  WHERE e.contact_id = $1
		    AND s.sent_at IS NOT NULL
		    AND s.sent_at > $2
		  ORDER BY s.sent_at DESC
		  LIMIT 1`,
		contactID, cutoff,
	).Scan(&lastSentAt)
	switch {
	case err == nil:
		return PreflightDecision{
			Verdict:    PreflightThrottled,
			StepIndex:  stepIndex,
			Email:      *email,
			Reason:     fmt.Sprintf("recipient throttled: a step was sent within %s", limits.ThrottleWindow),
			RetryAfter: lastSentAt.Add(limits.ThrottleWindow),
		}, nil
	case errors.Is(err, pgx.ErrNoRows):
		// not throttled — clear to send
	default:
		return PreflightDecision{}, fmt.Errorf("sequences: preflight throttle check %s: %w", enrollmentID, err)
	}

	return PreflightDecision{Verdict: PreflightAllow, StepIndex: stepIndex, Email: *email}, nil
}

// ErrNoRecipientEmail is returned by Preflight when the enrolled contact has no
// email address — an undeliverable enrollment. The send_step handler maps it to
// StateFailed (there is nothing to retry).
var ErrNoRecipientEmail = errors.New("sequences: enrollment contact has no email")

// PreflightVerdict is the category of a Preflight outcome.
type PreflightVerdict int

const (
	// PreflightAllow — every gate passed; the caller proceeds to the Brevo send.
	PreflightAllow PreflightVerdict = iota
	// PreflightSuppressed — the recipient is on the suppression list. Terminal:
	// the caller calls Transition(→ StateSuppressed) and does not send.
	PreflightSuppressed
	// PreflightCapped — the workspace monthly_send_cap is reached. Deferrable:
	// the caller reschedules send_step for RetryAfter (start of next month).
	PreflightCapped
	// PreflightThrottled — a step was sent to this recipient within the throttle
	// window. Deferrable: the caller reschedules send_step for RetryAfter.
	PreflightThrottled
)

// String renders the verdict for logs and audit payloads.
func (v PreflightVerdict) String() string {
	switch v {
	case PreflightAllow:
		return "allow"
	case PreflightSuppressed:
		return "suppressed"
	case PreflightCapped:
		return "capped"
	case PreflightThrottled:
		return "throttled"
	default:
		return fmt.Sprintf("PreflightVerdict(%d)", int(v))
	}
}

// PreflightDecision is the result of a Preflight evaluation. The send_step
// handler dispatches on Verdict:
//
//   - PreflightAllow     → send.
//   - PreflightSuppressed → Transition(ctx, tx, enrollmentID, StateSuppressed,
//     decision.Reason, …); no send.
//   - PreflightCapped / PreflightThrottled → do not send; reschedule the
//     send_step job for decision.RetryAfter (the enrollment stays enrolled).
type PreflightDecision struct {
	Verdict    PreflightVerdict
	StepIndex  int       // echoes the checked step, for the caller's audit payload
	Email      string    // resolved recipient (empty only on an error return)
	Reason     string    // human-readable; for Suppressed, the email_suppression.reason
	RetryAfter time.Time // when a deferrable verdict (capped/throttled) may retry; zero otherwise
}

// Allowed reports whether the send may proceed.
func (d PreflightDecision) Allowed() bool { return d.Verdict == PreflightAllow }

// Deferrable reports whether the verdict is a "try again later" outcome
// (capped or throttled) rather than allow or the terminal suppressed.
func (d PreflightDecision) Deferrable() bool {
	return d.Verdict == PreflightCapped || d.Verdict == PreflightThrottled
}

// Limits is the per-tenant deliverability budget Preflight enforces. The
// monthly cap is "config in workspace metadata" per ADR-004 rev 2 §8 / rev 1 §6;
// the worker runs as the workspace_<hex> role and cannot read core.workspaces,
// so the send_step handler supplies the per-tenant value (default DefaultLimits)
// rather than Preflight reading it. A zero field takes its default via
// withDefaults, so callers may set only the fields they override.
type Limits struct {
	// MonthlySendCap caps steps sent per calendar month, workspace-wide. <= 0
	// disables the cap (no limit). Phase defaults (rev 1 §6): phase 1 = 5,000,
	// phase 2 = 15,000, phase 3 = 30,000.
	MonthlySendCap int
	// ThrottleWindow is the minimum gap between two steps to the same contact_id.
	// Zero → DefaultThrottleWindow (24h).
	ThrottleWindow time.Duration
	// SoftBounceSuppressThreshold is the consecutive-soft-bounce count that
	// suppresses a recipient (bounce.go). Zero → DefaultSoftBounceSuppressThreshold.
	SoftBounceSuppressThreshold int
}

// Deliverability defaults (ADR-004 rev 2 §8, rev 1 §6).
const (
	// DefaultMonthlySendCap is the phase-1 per-tenant monthly cap (rev 1 §6).
	DefaultMonthlySendCap = 5000
	// DefaultThrottleWindow is the per-recipient minimum send gap (§8).
	DefaultThrottleWindow = 24 * time.Hour
	// DefaultSoftBounceSuppressThreshold is the consecutive-soft-bounce count
	// that suppresses a recipient (§8 bounce policy).
	DefaultSoftBounceSuppressThreshold = 3
)

// DefaultLimits returns the phase-1 deliverability budget. The send_step handler
// uses it as the baseline, overriding MonthlySendCap with the tenant's value
// when one is configured.
func DefaultLimits() Limits {
	return Limits{
		MonthlySendCap:              DefaultMonthlySendCap,
		ThrottleWindow:              DefaultThrottleWindow,
		SoftBounceSuppressThreshold: DefaultSoftBounceSuppressThreshold,
	}
}

// withDefaults fills zero-valued fields from the defaults. MonthlySendCap is left
// as-is (a caller may legitimately pass <= 0 to disable the cap); the time/count
// windows fall back so a partially-specified Limits is still safe.
func (l Limits) withDefaults() Limits {
	if l.ThrottleWindow <= 0 {
		l.ThrottleWindow = DefaultThrottleWindow
	}
	if l.SoftBounceSuppressThreshold <= 0 {
		l.SoftBounceSuppressThreshold = DefaultSoftBounceSuppressThreshold
	}
	return l
}

// startOfNextMonth returns midnight on the first day of the month after t, in
// t's location — when a capped workspace's budget resets.
func startOfNextMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m+1, 1, 0, 0, 0, 0, t.Location())
}
