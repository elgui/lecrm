package sequences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/gbconsult/lecrm/apps/api/capability"
)

// Transition performs one enrollment state transition (ADR-004 rev 2 §2). It is
// the single write path for the enrollment state machine; nothing else mutates
// enrollments.state. Every call, in order:
//
//  1. Locks the enrollment row (SELECT … FOR UPDATE) so concurrent workers for
//     the same enrollment serialize on it (ADR-004 rev 2 §2 step 1; this is the
//     row-level serialization that lets AcquireTx skip the workspace-wide
//     advisory lock — see db.TenantPool.AcquireTx).
//  2. Validates from → to against the in-code legalTransitions table (§2 step 2).
//     An illegal transition is a programming error: it panics in dev/test
//     (InvalidPanic, the default) and, in prod (InvalidAudit), emits a
//     sequences.transition.invalid audit row in-tx and returns an error the
//     caller surfaces as 500 (§2 last paragraph).
//  3. Updates enrollments.state, last_transition_at, and any side-effect columns
//     the caller set via options (reply_message_id, ooo_returns_at,
//     next_action_at, current_step_index) (§2 step 3).
//  4. Emits one core.audit_log row IN THE SAME TRANSACTION via the
//     SECURITY DEFINER function core.lecrm_emit_audit (migration 0026). The
//     emission is fail-closed (ADR-009 §7.2): if it errors, the caller's tx —
//     which already carries the state change — rolls back, so a state change
//     that cannot be audited never commits. The SDF is used (not a direct
//     INSERT) because the worker holds a workspace_<hex>-role connection, which
//     has no access to schema core at all (§2/§6; see 0026's header).
//  5. Enqueues sequences.finalize via river InsertTx when `to` is terminal and a
//     FinalizeEnqueuer is configured (§2 step 5, §3 "one finalize per
//     enrollment"). Because the job is inserted on the same tx, it becomes
//     visible iff the transition commits.
//
// tx MUST be a workspace-scoped transaction (db.TenantPool.AcquireTx). The
// caller owns the tx lifecycle — Transition never commits or rolls back; it
// returns an error so the worker's deferred release rolls back fail-closed.
func Transition(
	ctx context.Context,
	tx pgx.Tx,
	enrollmentID uuid.UUID,
	to State,
	reason string,
	opts ...Option,
) error {
	cfg := defaultConfig(to)
	for _, opt := range opts {
		opt(&cfg)
	}

	// 1. Lock the row and read the current state + the columns we need for the
	//    audit attribution and the finalize enqueue. enrollment_state is a
	//    per-workspace enum; scan it as text and convert to keep pgx protocol-
	//    agnostic (the tenant pool runs the simple query protocol).
	var (
		fromStr     string
		workspaceID uuid.UUID
	)
	err := tx.QueryRow(ctx,
		`SELECT state::text, workspace_id
		   FROM enrollments
		  WHERE id = $1
		  FOR UPDATE`,
		enrollmentID,
	).Scan(&fromStr, &workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrEnrollmentNotFound, enrollmentID)
	}
	if err != nil {
		return fmt.Errorf("sequences: lock enrollment %s: %w", enrollmentID, err)
	}
	from := State(fromStr)

	// 2. Validate. Illegal transitions are programming errors (§2).
	if !CanTransition(from, to) {
		invErr := &InvalidTransitionError{EnrollmentID: enrollmentID, From: from, To: to}
		if cfg.invalidMode == InvalidPanic {
			panic(invErr)
		}
		// Prod: trace the illegal transition (auth retention, §6) in-tx, then
		// return so the handler rolls back and surfaces 500.
		if aerr := emitAudit(ctx, tx, AuditEventTransitionInvalid, workspaceID, cfg,
			map[string]any{
				"enrollment_id": enrollmentID.String(),
				"from":          string(from),
				"to_attempted":  string(to),
				"caller":        cfg.caller,
			}); aerr != nil {
			return fmt.Errorf("%w (audit also failed: %v)", invErr, aerr)
		}
		return invErr
	}

	// 3. Apply the state change + any side-effect columns. Column names are
	//    compile-time literals (no injection surface); values are parameterized.
	setClauses := []string{"state = $2", "last_transition_at = now()"}
	args := []any{enrollmentID, string(to)}
	addCol := func(col string, val any) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, len(args)+1))
		args = append(args, val)
	}
	if cfg.replyMessageID != nil {
		addCol("reply_message_id", *cfg.replyMessageID)
	}
	if cfg.oooReturnsAt != nil {
		addCol("ooo_returns_at", *cfg.oooReturnsAt)
	}
	if cfg.nextActionAt != nil {
		addCol("next_action_at", *cfg.nextActionAt)
	}
	if cfg.currentStepIndex != nil {
		addCol("current_step_index", *cfg.currentStepIndex)
	}

	updateSQL := "UPDATE enrollments SET " + strings.Join(setClauses, ", ") + " WHERE id = $1"
	if _, err := tx.Exec(ctx, updateSQL, args...); err != nil {
		return fmt.Errorf("sequences: update enrollment %s state %s→%s: %w", enrollmentID, from, to, err)
	}

	// 4. Same-tx, fail-closed audit emission via the SDF.
	payload := map[string]any{
		"enrollment_id": enrollmentID.String(),
		"from":          string(from),
		"to":            string(to),
	}
	if reason != "" {
		payload["reason"] = reason
	}
	for k, v := range cfg.payload {
		payload[k] = v
	}
	if err := emitAudit(ctx, tx, cfg.event, workspaceID, cfg, payload); err != nil {
		return err
	}

	// 5. Enqueue the next river job the transition implies. v1: entering a
	//    terminal state enqueues sequences.finalize (one per enrollment via the
	//    args' UniqueOpts). Inserted on this tx so it appears iff we commit.
	if to.IsTerminal() && cfg.enqueuer != nil {
		if _, err := cfg.enqueuer.InsertTx(ctx, tx, FinalizeArgs{
			WorkspaceID:   workspaceID,
			EnrollmentID:  enrollmentID,
			TerminalState: string(to),
			Reason:        reason,
		}, nil); err != nil {
			return fmt.Errorf("sequences: enqueue finalize for %s: %w", enrollmentID, err)
		}
	}

	return nil
}

// emitAudit writes one core.audit_log row through the SECURITY DEFINER function
// core.lecrm_emit_audit (migration 0026), inside the caller's tx. This is the
// only audit path safe for a workspace_<hex>-role connection (the role has no
// direct access to schema core). Fail-closed: a returned error rolls the
// transition back.
//
// The jsonb payload is passed as a string, not []byte: under pgx's simple query
// protocol (the tenant pool's mode) a []byte is sent as a bytea literal and
// rejected by the jsonb parameter (SQLSTATE 22P02) — the same footgun avoided
// in capability.EmitAudit.
func emitAudit(ctx context.Context, tx pgx.Tx, event string, workspaceID uuid.UUID, cfg config, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sequences: audit marshal %s: %w", event, err)
	}
	if _, err := tx.Exec(ctx,
		`SELECT core.lecrm_emit_audit($1, $2, $3, $4, $5)`,
		event, workspaceID, cfg.actorType, cfg.actorUserID, string(body),
	); err != nil {
		return fmt.Errorf("sequences: audit emit %s: %w", event, err)
	}
	return nil
}

// --- errors ---

var (
	// ErrEnrollmentNotFound is returned when the enrollment row does not exist
	// (or was deleted before the lock). REST/worker callers map it to 404/discard.
	ErrEnrollmentNotFound = errors.New("sequences: enrollment not found")

	// ErrInvalidTransition is the sentinel an InvalidTransitionError matches via
	// errors.Is, so callers can switch on it without unwrapping the concrete type.
	ErrInvalidTransition = errors.New("sequences: invalid transition")
)

// InvalidTransitionError describes an illegal from → to transition (a
// programming error, ADR-004 rev 2 §2). In InvalidPanic mode it is the panic
// value; in InvalidAudit mode it is returned after the sequences.transition.invalid
// audit row is emitted.
type InvalidTransitionError struct {
	EnrollmentID uuid.UUID
	From         State
	To           State
}

func (e *InvalidTransitionError) Error() string {
	return fmt.Sprintf("sequences: invalid transition %s → %s for enrollment %s (ADR-004 rev 2 §2)",
		e.From, e.To, e.EnrollmentID)
}

// Is reports a match for the ErrInvalidTransition sentinel.
func (e *InvalidTransitionError) Is(target error) bool { return target == ErrInvalidTransition }

// --- finalize enqueue seam ---

// FinalizeEnqueuer inserts a river job on an existing transaction (ADR-004 rev 2
// §2 step 5). *river.Client[pgx.Tx] satisfies it directly; tests substitute a
// recording stub. Declared here so Transition does not have to hold a concrete
// river client — the worker wiring injects one.
type FinalizeEnqueuer interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Compile-time proof that the production river client satisfies the seam.
var _ FinalizeEnqueuer = (*river.Client[pgx.Tx])(nil)

// --- invalid-transition policy ---

// InvalidMode selects how Transition treats an illegal transition (ADR-004 rev 2
// §2: "they panic in dev/test and return 500 … in prod").
type InvalidMode int

const (
	// InvalidPanic panics on an illegal transition. Default, for dev/test: an
	// illegal transition is a bug that should fail loudly and abort the test.
	InvalidPanic InvalidMode = iota
	// InvalidAudit emits sequences.transition.invalid in-tx and returns an
	// InvalidTransitionError instead of panicking. Production wiring selects it
	// so a stray illegal transition degrades to a 500 + audit trail, not a crash.
	InvalidAudit
)

// eventTransitionGeneric is the default audit event for transitions into a state
// ADR-004 rev 2 §6 does not name a distinct event for (waiting_reply,
// suppressed, completed). §6 / audit.go delegate this naming to the state-machine
// tasket; callers may override per-call with WithEvent.
const eventTransitionGeneric = "sequences.transition"

// --- options ---

type config struct {
	actorType        string
	actorUserID      uuid.NullUUID
	event            string
	payload          map[string]any
	replyMessageID   *string
	oooReturnsAt     *time.Time
	nextActionAt     *time.Time
	currentStepIndex *int
	caller           string
	invalidMode      InvalidMode
	enqueuer         FinalizeEnqueuer
}

func defaultConfig(to State) config {
	return config{
		// Engine-emitted transitions attribute to internal_service
		// (ADR-004 rev 2 §6 / ADR-009 §4.1). The constant is the single source
		// of truth in apps/api/capability, not duplicated here.
		actorType:   capability.ActorTypeInternalService,
		event:       defaultEventForState(to),
		invalidMode: InvalidPanic,
	}
}

// defaultEventForState maps a target state to its ADR-004 rev 2 §6 audit event,
// falling back to the generic transition event for the states §6 leaves unnamed.
func defaultEventForState(to State) string {
	switch to {
	case StateEnrolled:
		return AuditEventEnrolled
	case StateStepSent:
		return AuditEventStepSent
	case StateReplyReceived:
		return AuditEventReplyReceived
	case StateOOODetected:
		return AuditEventOOODetected
	case StateFailed:
		return AuditEventFailed
	case StateBounced:
		return AuditEventBounced
	case StateUnsubscribed:
		return AuditEventUnsubscribed
	default:
		return eventTransitionGeneric
	}
}

// Option configures a single Transition call.
type Option func(*config)

// WithActor overrides the audit actor attribution. actorType defaults to
// capability.ActorTypeInternalService (engine-emitted); pass human_api/mcp_agent
// for a user- or agent-initiated transition. A zero actorUserID is recorded as
// NULL.
func WithActor(actorType string, actorUserID uuid.UUID) Option {
	return func(c *config) {
		if actorType != "" {
			c.actorType = actorType
		}
		c.actorUserID = uuid.NullUUID{UUID: actorUserID, Valid: actorUserID != uuid.Nil}
	}
}

// WithEvent overrides the audit event name (default: the ADR-004 rev 2 §6 event
// for the target state). Use it for transitions §6 leaves unnamed, e.g. a
// waiting_reply→enrolled resume that should read as sequences.step_sent.
func WithEvent(event string) Option {
	return func(c *config) {
		if event != "" {
			c.event = event
		}
	}
}

// WithPayload merges extra fields into the audit payload (ADR-004 rev 2 §6
// per-event fields, e.g. step_index, brevo_message_id). enrollment_id/from/to/
// reason are always present and are not overridden unless the caller sets them.
func WithPayload(payload map[string]any) Option {
	return func(c *config) { c.payload = payload }
}

// WithReplyMessageID sets enrollments.reply_message_id (§2 step 3 side-effect).
func WithReplyMessageID(id string) Option {
	return func(c *config) { c.replyMessageID = &id }
}

// WithOOOReturnsAt sets enrollments.ooo_returns_at (§2 step 3 / §5).
func WithOOOReturnsAt(t time.Time) Option {
	return func(c *config) { c.oooReturnsAt = &t }
}

// WithNextActionAt sets enrollments.next_action_at — when the next send_step job
// is scheduled (§2 step 3).
func WithNextActionAt(t time.Time) Option {
	return func(c *config) { c.nextActionAt = &t }
}

// WithCurrentStepIndex sets enrollments.current_step_index — used when a resume
// (waiting_reply/ooo_detected → enrolled) advances to the next step.
func WithCurrentStepIndex(idx int) Option {
	return func(c *config) { c.currentStepIndex = &idx }
}

// WithCaller records the caller in the sequences.transition.invalid audit
// payload (§6) so an illegal transition is traceable to the worker that
// attempted it.
func WithCaller(caller string) Option {
	return func(c *config) { c.caller = caller }
}

// WithInvalidMode selects the illegal-transition policy (default InvalidPanic).
// Production wiring passes InvalidAudit.
func WithInvalidMode(m InvalidMode) Option {
	return func(c *config) { c.invalidMode = m }
}

// WithFinalizeEnqueuer wires the river client used to enqueue sequences.finalize
// when the transition enters a terminal state. Omit it (or pass nil) to skip the
// enqueue — e.g. unit tests, or a caller that finalizes separately.
func WithFinalizeEnqueuer(e FinalizeEnqueuer) Option {
	return func(c *config) { c.enqueuer = e }
}
