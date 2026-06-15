package sequences

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Bounce policy (ADR-004 rev 2 §8, carried from rev 1 §5):
//
//   - A hard bounce or a complaint suppresses the recipient immediately — one
//     event is a permanent signal.
//   - A soft bounce is transient and does NOT suppress on its own; only after
//     SoftBounceSuppressThreshold (default 3) CONSECUTIVE soft bounces to the
//     same address — with no successful delivery in between — is the recipient
//     treated as undeliverable and suppressed.
//
// This is the write-side counterpart to Preflight's read-side suppression gate:
// the bounce-webhook path records the bounce on the enrollment_steps row, then
// calls ApplyBounce, which decides and (when warranted) writes the
// email_suppression row that a later Preflight will read and act on.
//
// The suppression reasons MUST be one of the four values allowed by the
// email_suppression.reason CHECK constraint
// (packages/db/migrations/0025 step 13): hard_bounce, blocked, complaint,
// unsubscribed. A soft-bounce-threshold suppression has no dedicated reason, so
// it is recorded as hard_bounce — by that point the address is, for our
// purposes, permanently undeliverable; the nuance (it was N soft bounces, not a
// single hard one) lives in the sequences.bounced audit payload, not the
// suppression row.

// BounceKind is the class of a bounce/complaint event as the engine sees it.
// It maps onto Brevo's event vocabulary (apps/api/internal/email/brevo) but is
// declared here so the sequences package does not depend on the email package.
type BounceKind string

const (
	// BounceSoft is a transient failure (mailbox full, greylisting, temporary
	// DNS). Suppresses only after the consecutive threshold.
	BounceSoft BounceKind = "soft"
	// BounceHard is a permanent failure (no such mailbox/domain). Immediate
	// suppression.
	BounceHard BounceKind = "hard"
	// BounceComplaint is a spam complaint (FBL / Brevo `spam`). Immediate
	// suppression — the most reputation-damaging signal.
	BounceComplaint BounceKind = "complaint"
)

// Suppression reasons written to email_suppression.reason. These MUST stay in
// sync with the column CHECK (migration 0025 step 13) and with the email
// package's brevo.SuppressionReason mapping.
const (
	SuppressionReasonHardBounce   = "hard_bounce"
	SuppressionReasonComplaint    = "complaint"
	SuppressionReasonBlocked      = "blocked"
	SuppressionReasonUnsubscribed = "unsubscribed"
)

// BounceAction is the decision EvaluateBounce returns: whether to suppress the
// recipient and, if so, under which reason.
type BounceAction struct {
	Suppress bool
	Reason   string // a Suppression* reason; empty when Suppress is false
}

// EvaluateBounce is the pure bounce-policy decision (no I/O). Given the kind of
// the just-observed bounce and the recipient's CONSECUTIVE soft-bounce count
// (which, for a soft bounce, must already include the current event — see
// ApplyBounce), it returns whether the recipient should be suppressed.
//
// Hard/complaint suppress on the single event. Soft suppresses only once the
// consecutive run reaches limits.SoftBounceSuppressThreshold.
func EvaluateBounce(kind BounceKind, consecutiveSoftBounces int, limits Limits) BounceAction {
	limits = limits.withDefaults()
	switch kind {
	case BounceHard:
		return BounceAction{Suppress: true, Reason: SuppressionReasonHardBounce}
	case BounceComplaint:
		return BounceAction{Suppress: true, Reason: SuppressionReasonComplaint}
	case BounceSoft:
		if consecutiveSoftBounces >= limits.SoftBounceSuppressThreshold {
			return BounceAction{Suppress: true, Reason: SuppressionReasonHardBounce}
		}
		return BounceAction{Suppress: false}
	default:
		return BounceAction{Suppress: false}
	}
}

// ConsecutiveSoftBounces returns the length of the current trailing run of soft
// bounces for email — the number of most-recent send attempts to that address
// that soft-bounced with NO successful delivery (or hard bounce) interrupting
// the run. A delivered/sent-OK attempt resets the run to zero, which is what
// makes the threshold "consecutive" rather than cumulative.
//
// The run is computed server-side in one query so the check stays a single
// round-trip on the workspace tx. Attempts are ordered most-recent-first by
// sent_at; the run is the number of rows before the first non-soft-bounce
// attempt (or the full set when every attempt soft-bounced).
func ConsecutiveSoftBounces(ctx context.Context, tx pgx.Tx, email string) (int, error) {
	var n int
	err := tx.QueryRow(ctx,
		`WITH ordered AS (
		   SELECT (s.bounced_at IS NOT NULL AND s.bounce_type = 'soft') AS is_soft,
		          row_number() OVER (ORDER BY s.sent_at DESC) AS rn
		     FROM enrollment_steps s
		     JOIN enrollments e ON e.id = s.enrollment_id
		     JOIN contacts c ON c.id = e.contact_id
		    WHERE c.email = $1
		      AND s.sent_at IS NOT NULL
		 )
		 SELECT COALESCE(
		   (SELECT min(rn) - 1 FROM ordered WHERE NOT is_soft),
		   (SELECT count(*) FROM ordered)
		 )::int`,
		email,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("sequences: consecutive soft bounces for %s: %w", email, err)
	}
	return n, nil
}

// ApplyBounce evaluates the bounce policy for a recipient and, when it warrants
// suppression, writes the email_suppression row — all on the caller's
// workspace-scoped tx. The caller (bounce-webhook handler) MUST first record the
// current bounce on the enrollment_steps row (state, bounce_type, bounced_at) so
// that, for a soft bounce, the ConsecutiveSoftBounces count includes it.
//
// Returns the BounceAction taken so the caller can emit the matching
// sequences.bounced audit row and, for a suppression, drive
// Transition(→ StateBounced/StateSuppressed).
func ApplyBounce(ctx context.Context, tx pgx.Tx, email string, kind BounceKind, limits Limits) (BounceAction, error) {
	limits = limits.withDefaults()

	var consecutive int
	if kind == BounceSoft {
		var err error
		consecutive, err = ConsecutiveSoftBounces(ctx, tx, email)
		if err != nil {
			return BounceAction{}, err
		}
	}

	action := EvaluateBounce(kind, consecutive, limits)
	if !action.Suppress {
		return action, nil
	}
	if err := upsertSuppression(ctx, tx, email, action.Reason); err != nil {
		return action, err
	}
	return action, nil
}

// upsertSuppression writes (or refreshes) the email_suppression row for email on
// the workspace tx. The (email) UNIQUE constraint makes it idempotent across
// replayed bounce deliveries. Unqualified table name resolves via the workspace
// role's search_path, mirroring email.PgSuppressionStore.Upsert (which runs on
// the main pool with an explicit schema).
func upsertSuppression(ctx context.Context, tx pgx.Tx, email, reason string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO email_suppression (email, reason, suppressed_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (email) DO UPDATE
		   SET reason = EXCLUDED.reason, suppressed_at = EXCLUDED.suppressed_at`,
		email, reason,
	)
	if err != nil {
		return fmt.Errorf("sequences: suppression upsert for %s: %w", email, err)
	}
	return nil
}
