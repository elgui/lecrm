package sequences

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/gbconsult/lecrm/apps/api/capability"
)

// recordedExec captures one tx.Exec call so a test can assert what SQL the
// state machine issued and with which parameters.
type recordedExec struct {
	sql  string
	args []any
}

// transitionTx is a pgx.Tx test double for Transition. It answers the
// SELECT … FOR UPDATE with canned (state, workspace_id) values and records every
// Exec so a test can prove the UPDATE shape and the SDF audit call without a
// database. Embedding pgx.Tx (nil) means any method the function calls that the
// test did not stub nil-derefs — surfacing an unexpected DB touch as a failure.
type transitionTx struct {
	pgx.Tx
	fromState string
	wsID      uuid.UUID
	noRow     bool

	execs     []recordedExec
	updateErr error // returned by the UPDATE enrollments Exec
	auditErr  error // returned by the SELECT core.lecrm_emit_audit Exec
}

func (t *transitionTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if t.noRow {
		return errRow{err: pgx.ErrNoRows}
	}
	return stateRow{state: t.fromState, wsID: t.wsID}
}

func (t *transitionTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	t.execs = append(t.execs, recordedExec{sql: sql, args: args})
	switch {
	case strings.Contains(sql, "UPDATE enrollments"):
		return pgconn.NewCommandTag("UPDATE 1"), t.updateErr
	case strings.Contains(sql, "lecrm_emit_audit"):
		return pgconn.NewCommandTag("SELECT 1"), t.auditErr
	default:
		return pgconn.CommandTag{}, nil
	}
}

func (t *transitionTx) execMatching(substr string) (recordedExec, bool) {
	for _, e := range t.execs {
		if strings.Contains(e.sql, substr) {
			return e, true
		}
	}
	return recordedExec{}, false
}

// stateRow scans (state text, workspace_id uuid) into Transition's locals.
type stateRow struct {
	state string
	wsID  uuid.UUID
}

func (r stateRow) Scan(dest ...any) error {
	if len(dest) != 2 {
		return errors.New("stateRow: unexpected dest count")
	}
	*(dest[0].(*string)) = r.state
	*(dest[1].(*uuid.UUID)) = r.wsID
	return nil
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

// recordingEnqueuer records every finalize job inserted on the tx.
type recordingEnqueuer struct {
	calls []river.JobArgs
	err   error
}

func (e *recordingEnqueuer) InsertTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	e.calls = append(e.calls, args)
	return &rivertype.JobInsertResult{}, e.err
}

// TestTransition_ValidNonTerminal_UpdatesAndAudits: a legal non-terminal
// transition issues exactly one UPDATE and one SDF audit emission, with the
// §6 event for the target state, and enqueues NO finalize (the target is not
// terminal).
func TestTransition_ValidNonTerminal_UpdatesAndAudits(t *testing.T) {
	tx := &transitionTx{fromState: string(StateEnrolled), wsID: wsA}
	enq := &recordingEnqueuer{}

	err := Transition(context.Background(), tx, enrA, StateStepSent, "send ok",
		WithFinalizeEnqueuer(enq))
	if err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}

	upd, ok := tx.execMatching("UPDATE enrollments")
	if !ok {
		t.Fatal("no UPDATE enrollments issued")
	}
	if !strings.Contains(upd.sql, "last_transition_at = now()") {
		t.Errorf("UPDATE does not set last_transition_at: %q", upd.sql)
	}
	if got := upd.args[1]; got != string(StateStepSent) {
		t.Errorf("UPDATE state param = %v, want %q", got, StateStepSent)
	}

	audit, ok := tx.execMatching("lecrm_emit_audit")
	if !ok {
		t.Fatal("no audit emission issued")
	}
	if got := audit.args[0]; got != AuditEventStepSent {
		t.Errorf("audit event = %v, want %q", got, AuditEventStepSent)
	}
	if len(enq.calls) != 0 {
		t.Errorf("finalize enqueued for a non-terminal transition: %+v", enq.calls)
	}
}

// TestTransition_WorkerRoleSafeAuditPath: the audit row is written through the
// SECURITY DEFINER function core.lecrm_emit_audit — NOT a direct
// INSERT INTO core.audit_log — because the worker holds a workspace_<hex> role
// with no access to schema core (ADR-004 rev 2 §6, migration 0026). The actor
// is internal_service and the payload is sent as a JSON string (jsonb
// simple-protocol footgun), carrying from/to/reason.
func TestTransition_WorkerRoleSafeAuditPath(t *testing.T) {
	tx := &transitionTx{fromState: string(StateStepSent), wsID: wsA}

	if err := Transition(context.Background(), tx, enrA, StateWaitingReply, "entering window"); err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}

	audit, ok := tx.execMatching("lecrm_emit_audit")
	if !ok {
		t.Fatal("no audit emission issued")
	}
	if _, direct := tx.execMatching("INSERT INTO core.audit_log"); direct {
		t.Error("audit written via direct core.audit_log INSERT — illegal for a workspace role")
	}
	// arg order: event, workspace_id, actor_type, actor_user_id, payload(json string)
	if got := audit.args[2]; got != capability.ActorTypeInternalService {
		t.Errorf("audit actor_type = %v, want %q", got, capability.ActorTypeInternalService)
	}
	if got, ok := audit.args[1].(uuid.UUID); !ok || got != wsA {
		t.Errorf("audit workspace_id = %v, want %s", audit.args[1], wsA)
	}
	body, ok := audit.args[4].(string)
	if !ok {
		t.Fatalf("audit payload param is %T, want string (jsonb simple-protocol footgun)", audit.args[4])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("audit payload is not valid JSON: %v", err)
	}
	if payload["from"] != string(StateStepSent) || payload["to"] != string(StateWaitingReply) {
		t.Errorf("audit payload from/to = %v/%v, want %s/%s", payload["from"], payload["to"], StateStepSent, StateWaitingReply)
	}
	if payload["reason"] != "entering window" {
		t.Errorf("audit payload reason = %v, want %q", payload["reason"], "entering window")
	}
}

// TestTransition_ValidTerminal_EnqueuesFinalize: entering a terminal state
// enqueues exactly one sequences.finalize carrying the terminal state + reason,
// on the same tx.
func TestTransition_ValidTerminal_EnqueuesFinalize(t *testing.T) {
	tx := &transitionTx{fromState: string(StateStepSent), wsID: wsA}
	enq := &recordingEnqueuer{}

	if err := Transition(context.Background(), tx, enrA, StateCompleted, "all steps sent",
		WithFinalizeEnqueuer(enq)); err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}

	if len(enq.calls) != 1 {
		t.Fatalf("finalize enqueued %d times, want 1", len(enq.calls))
	}
	fin, ok := enq.calls[0].(FinalizeArgs)
	if !ok {
		t.Fatalf("enqueued args type = %T, want FinalizeArgs", enq.calls[0])
	}
	if fin.EnrollmentID != enrA || fin.WorkspaceID != wsA {
		t.Errorf("finalize args ids = (%s,%s), want (%s,%s)", fin.WorkspaceID, fin.EnrollmentID, wsA, enrA)
	}
	if fin.TerminalState != string(StateCompleted) || fin.Reason != "all steps sent" {
		t.Errorf("finalize args = (%q,%q), want (%q,%q)", fin.TerminalState, fin.Reason, StateCompleted, "all steps sent")
	}
}

// TestTransition_NoEnqueuer_TerminalStillSucceeds: a terminal transition without
// a configured enqueuer applies the state change and audit but skips the
// enqueue (no panic, no error).
func TestTransition_NoEnqueuer_TerminalStillSucceeds(t *testing.T) {
	tx := &transitionTx{fromState: string(StateStepSent), wsID: wsA}
	if err := Transition(context.Background(), tx, enrA, StateBounced, "hard bounce"); err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}
	if _, ok := tx.execMatching("UPDATE enrollments"); !ok {
		t.Error("terminal transition without enqueuer skipped the UPDATE")
	}
}

// TestTransition_InvalidPanics: in the default (dev/test) mode an illegal
// transition panics with an *InvalidTransitionError before any write.
func TestTransition_InvalidPanics(t *testing.T) {
	tx := &transitionTx{fromState: string(StateEnrolled), wsID: wsA}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Transition did not panic on an illegal transition")
		}
		var ite *InvalidTransitionError
		if !errors.As(r.(error), &ite) {
			t.Fatalf("panic value = %v (%T), want *InvalidTransitionError", r, r)
		}
		if ite.From != StateEnrolled || ite.To != StateReplyReceived {
			t.Errorf("panic from/to = %s/%s, want enrolled/reply_received", ite.From, ite.To)
		}
		// No writes before the panic.
		if len(tx.execs) != 0 {
			t.Errorf("Transition wrote %d times before panicking: %+v", len(tx.execs), tx.execs)
		}
	}()

	// enrolled → reply_received is illegal (must pass through step_sent/waiting_reply).
	_ = Transition(context.Background(), tx, enrA, StateReplyReceived, "bug")
}

// TestTransition_InvalidAudit_EmitsAndReturns: in prod mode an illegal
// transition emits sequences.transition.invalid (carrying from/to_attempted/
// caller) in-tx and returns an ErrInvalidTransition — no UPDATE.
func TestTransition_InvalidAudit_EmitsAndReturns(t *testing.T) {
	tx := &transitionTx{fromState: string(StateEnrolled), wsID: wsA}

	err := Transition(context.Background(), tx, enrA, StateReplyReceived, "bug",
		WithInvalidMode(InvalidAudit), WithCaller("poll_reply_worker"))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Transition error = %v, want ErrInvalidTransition", err)
	}
	if _, ok := tx.execMatching("UPDATE enrollments"); ok {
		t.Error("illegal transition issued an UPDATE in audit mode")
	}
	audit, ok := tx.execMatching("lecrm_emit_audit")
	if !ok {
		t.Fatal("no sequences.transition.invalid audit emitted")
	}
	if got := audit.args[0]; got != AuditEventTransitionInvalid {
		t.Errorf("audit event = %v, want %q", got, AuditEventTransitionInvalid)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(audit.args[4].(string)), &payload)
	if payload["to_attempted"] != string(StateReplyReceived) {
		t.Errorf("audit to_attempted = %v, want %q", payload["to_attempted"], StateReplyReceived)
	}
	if payload["caller"] != "poll_reply_worker" {
		t.Errorf("audit caller = %v, want %q", payload["caller"], "poll_reply_worker")
	}
}

// TestTransition_EnrollmentNotFound: a missing enrollment row maps to
// ErrEnrollmentNotFound with no writes.
func TestTransition_EnrollmentNotFound(t *testing.T) {
	tx := &transitionTx{noRow: true}
	err := Transition(context.Background(), tx, enrA, StateStepSent, "x")
	if !errors.Is(err, ErrEnrollmentNotFound) {
		t.Fatalf("Transition error = %v, want ErrEnrollmentNotFound", err)
	}
	if len(tx.execs) != 0 {
		t.Errorf("Transition wrote despite missing enrollment: %+v", tx.execs)
	}
}

// TestTransition_FailClosedOnAuditError: if the in-tx audit emission fails,
// Transition returns the error so the caller's deferred rollback discards the
// already-applied state change (ADR-009 §7.2 fail-closed). No finalize is
// enqueued past the failed audit.
func TestTransition_FailClosedOnAuditError(t *testing.T) {
	auditBoom := errors.New("permission denied for schema core")
	tx := &transitionTx{fromState: string(StateStepSent), wsID: wsA, auditErr: auditBoom}
	enq := &recordingEnqueuer{}

	err := Transition(context.Background(), tx, enrA, StateCompleted, "done",
		WithFinalizeEnqueuer(enq))
	if !errors.Is(err, auditBoom) {
		t.Fatalf("Transition error = %v, want the audit error", err)
	}
	if len(enq.calls) != 0 {
		t.Errorf("finalize enqueued after a failed audit (not fail-closed): %+v", enq.calls)
	}
}

// TestTransition_SideEffectColumns: WithReplyMessageID / WithOOOReturnsAt /
// WithNextActionAt put the side-effect columns (ADR-004 rev 2 §2 step 3) into
// the UPDATE with parameterized values.
func TestTransition_SideEffectColumns(t *testing.T) {
	tx := &transitionTx{fromState: string(StateWaitingReply), wsID: wsA}
	when := time.Now().Add(48 * time.Hour)

	if err := Transition(context.Background(), tx, enrA, StateOOODetected, "ooo",
		WithReplyMessageID("<msg-1@mail>"),
		WithOOOReturnsAt(when),
	); err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}

	upd, _ := tx.execMatching("UPDATE enrollments")
	for _, col := range []string{"reply_message_id", "ooo_returns_at"} {
		if !strings.Contains(upd.sql, col) {
			t.Errorf("UPDATE missing side-effect column %q: %q", col, upd.sql)
		}
	}
	// args: [enrollmentID, state, reply_message_id, ooo_returns_at]
	if len(upd.args) != 4 {
		t.Fatalf("UPDATE arg count = %d, want 4 (%v)", len(upd.args), upd.args)
	}
	if upd.args[2] != "<msg-1@mail>" {
		t.Errorf("reply_message_id param = %v, want <msg-1@mail>", upd.args[2])
	}
}
