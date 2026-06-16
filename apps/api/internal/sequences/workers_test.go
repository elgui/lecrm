package sequences

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// fakeTx is a no-op pgx.Tx that only records Commit/Rollback. It embeds the
// pgx.Tx interface (left nil) so every other method is promoted; the workers
// under test never call those, so a nil-deref would itself be a test failure
// signalling the worker touched the connection unexpectedly.
type fakeTx struct {
	pgx.Tx
	committed  bool
	rolledBack bool
}

func (t *fakeTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(context.Context) error {
	if t.committed {
		return pgx.ErrTxClosed // mirrors real pgx: rollback after commit is a no-op error
	}
	t.rolledBack = true
	return nil
}

// stubAcquirer implements WorkspaceTxAcquirer without a database. It hands out
// a fresh fakeTx per AcquireTx call and records the workspace IDs it saw and
// whether each release ran.
type stubAcquirer struct {
	acquireErr error
	txs        []*fakeTx
	gotWSIDs   []uuid.UUID
	releases   int
}

func (s *stubAcquirer) AcquireTx(ctx context.Context, wsID uuid.UUID) (context.Context, pgx.Tx, func(), error) {
	if s.acquireErr != nil {
		return ctx, nil, nil, s.acquireErr
	}
	s.gotWSIDs = append(s.gotWSIDs, wsID)
	tx := &fakeTx{}
	s.txs = append(s.txs, tx)
	release := func() {
		s.releases++
		_ = tx.Rollback(ctx)
	}
	return ctx, tx, release, nil
}

func (s *stubAcquirer) last() *fakeTx { return s.txs[len(s.txs)-1] }

const testStep = 2

func sendJob(attempt, maxAttempts int) *river.Job[SendStepArgs] {
	return &river.Job[SendStepArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: maxAttempts},
		Args:   SendStepArgs{WorkspaceID: wsA, EnrollmentID: enrA, StepIndex: testStep},
	}
}

// TestRegisterWorkers_RegistersFourDistinctKinds asserts RegisterWorkers wires
// all four workers without panicking. river.AddWorker panics on a duplicate
// kind, so a clean return proves the four args types map to four distinct
// kinds (the kind-routing keys).
func TestRegisterWorkers_RegistersFourDistinctKinds(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterWorkers panicked (kind collision or invalid worker?): %v", r)
		}
	}()
	workers := river.NewWorkers()
	got := RegisterWorkers(workers, &stubAcquirer{}, Handlers{})
	if got != workers {
		t.Fatalf("RegisterWorkers returned a different bundle than it was given")
	}
}

// TestWorker_AcquireTxContract_CommitsOnSuccess verifies the core contract: a
// worker acquires a workspace-scoped tx, runs the handler against THAT tx, and
// commits on success. It also confirms the tx is scoped to the args' workspace.
func TestWorker_AcquireTxContract_CommitsOnSuccess(t *testing.T) {
	acq := &stubAcquirer{}
	var sawTx pgx.Tx
	handlerRan := false
	w := &enrollWorker{
		acq: acq,
		handle: func(_ context.Context, tx pgx.Tx, args EnrollArgs) error {
			handlerRan = true
			sawTx = tx
			if args.WorkspaceID != wsA {
				t.Errorf("handler got workspace %s, want %s", args.WorkspaceID, wsA)
			}
			return nil
		},
	}
	job := &river.Job[EnrollArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   EnrollArgs{WorkspaceID: wsA, ContactID: enrA, SequenceID: enrB},
	}

	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}
	if !handlerRan {
		t.Fatal("handler was never invoked")
	}
	if sawTx != acq.last() {
		t.Fatal("handler did not receive the acquired tx")
	}
	if !acq.last().committed {
		t.Error("tx was not committed on success")
	}
	if acq.releases != 1 {
		t.Errorf("release ran %d times, want exactly 1", acq.releases)
	}
	if len(acq.gotWSIDs) != 1 || acq.gotWSIDs[0] != wsA {
		t.Errorf("AcquireTx workspace ids = %v, want [%s]", acq.gotWSIDs, wsA)
	}
}

// TestWorker_AcquireTxContract_RollsBackOnHandlerError verifies that a handler
// error propagates (so river retries) and the tx is NOT committed — the
// deferred release rolls it back.
func TestWorker_AcquireTxContract_RollsBackOnHandlerError(t *testing.T) {
	acq := &stubAcquirer{}
	wantErr := errors.New("boom")
	w := &pollReplyWorker{
		acq:    acq,
		handle: func(context.Context, pgx.Tx, PollReplyArgs) error { return wantErr },
	}
	job := &river.Job[PollReplyArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   PollReplyArgs{WorkspaceID: wsA, EnrollmentID: enrA},
	}

	err := w.Work(context.Background(), job)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Work error = %v, want %v", err, wantErr)
	}
	if acq.last().committed {
		t.Error("tx was committed despite handler error")
	}
	if !acq.last().rolledBack {
		t.Error("tx was not rolled back after handler error")
	}
	if acq.releases != 1 {
		t.Errorf("release ran %d times, want exactly 1", acq.releases)
	}
}

// TestWorker_UnwiredHandlerReturnsSentinel verifies that a worker with no
// handler returns ErrHandlerNotWired (job stays retryable) rather than
// silently completing.
func TestWorker_UnwiredHandlerReturnsSentinel(t *testing.T) {
	acq := &stubAcquirer{}
	w := &finalizeWorker{acq: acq} // handle == nil
	job := &river.Job[FinalizeArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   FinalizeArgs{WorkspaceID: wsA, EnrollmentID: enrA, TerminalState: string(StateCompleted)},
	}

	err := w.Work(context.Background(), job)
	if !errors.Is(err, ErrHandlerNotWired) {
		t.Fatalf("Work error = %v, want ErrHandlerNotWired", err)
	}
	if acq.last().committed {
		t.Error("unwired worker should not commit")
	}
}

// TestWorker_PropagatesAcquireError verifies that if AcquireTx itself fails,
// the worker returns that error and never runs the handler.
func TestWorker_PropagatesAcquireError(t *testing.T) {
	wantErr := errors.New("pool exhausted")
	acq := &stubAcquirer{acquireErr: wantErr}
	handlerRan := false
	w := &enrollWorker{
		acq:    acq,
		handle: func(context.Context, pgx.Tx, EnrollArgs) error { handlerRan = true; return nil },
	}
	job := &river.Job[EnrollArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   EnrollArgs{WorkspaceID: wsA},
	}

	if err := w.Work(context.Background(), job); !errors.Is(err, wantErr) {
		t.Fatalf("Work error = %v, want %v", err, wantErr)
	}
	if handlerRan {
		t.Error("handler ran despite AcquireTx failure")
	}
}

// TestSendStepWorker_ExhaustedTransitionOnFinalAttempt verifies the
// "5 → failed" contract: on the final attempt, after the send handler errors,
// the onExhausted hook fires in a fresh committed tx, and the original send
// error is still propagated (so river discards the job).
func TestSendStepWorker_ExhaustedTransitionOnFinalAttempt(t *testing.T) {
	acq := &stubAcquirer{}
	sendErr := errors.New("provider 5xx")
	exhaustedRan := false
	var gotCause error
	w := &sendStepWorker{
		acq:    acq,
		handle: func(context.Context, pgx.Tx, SendStepArgs) error { return sendErr },
		onExhausted: func(_ context.Context, _ pgx.Tx, args SendStepArgs, cause error) error {
			exhaustedRan = true
			gotCause = cause
			if args.StepIndex != testStep {
				t.Errorf("exhausted hook got step %d, want %d", args.StepIndex, testStep)
			}
			return nil
		},
	}

	err := w.Work(context.Background(), sendJob(maxAttemptsSendStep, maxAttemptsSendStep))
	if !errors.Is(err, sendErr) {
		t.Fatalf("Work error = %v, want the send error %v", err, sendErr)
	}
	if !exhaustedRan {
		t.Fatal("onExhausted did not run on the final attempt")
	}
	if !errors.Is(gotCause, sendErr) {
		t.Errorf("onExhausted cause = %v, want %v", gotCause, sendErr)
	}
	// Two AcquireTx calls: the failed send tx, then the exhausted-transition tx.
	if len(acq.txs) != 2 {
		t.Fatalf("expected 2 tx acquisitions (send + exhausted), got %d", len(acq.txs))
	}
	if acq.txs[0].committed {
		t.Error("the failed send tx must not be committed")
	}
	if !acq.txs[1].committed {
		t.Error("the exhausted-transition tx must be committed")
	}
}

// TestSendStepWorker_NoExhaustedTransitionBeforeFinalAttempt verifies that a
// non-final failure just retries (no terminal transition), so an intermittent
// provider error does not prematurely fail the enrollment.
func TestSendStepWorker_NoExhaustedTransitionBeforeFinalAttempt(t *testing.T) {
	acq := &stubAcquirer{}
	exhaustedRan := false
	w := &sendStepWorker{
		acq:         acq,
		handle:      func(context.Context, pgx.Tx, SendStepArgs) error { return errors.New("transient") },
		onExhausted: func(context.Context, pgx.Tx, SendStepArgs, error) error { exhaustedRan = true; return nil },
	}

	// Attempt 1 of 5 — not the final attempt.
	_ = w.Work(context.Background(), sendJob(1, maxAttemptsSendStep))
	if exhaustedRan {
		t.Error("onExhausted ran on a non-final attempt")
	}
	if len(acq.txs) != 1 {
		t.Errorf("expected exactly 1 tx (the send attempt), got %d", len(acq.txs))
	}
}
