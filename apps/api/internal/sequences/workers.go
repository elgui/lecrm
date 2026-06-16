package sequences

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// ErrHandlerNotWired is returned by a worker whose domain handler has not yet
// been injected. The river framework + AcquireTx envelope is this tasket's
// deliverable (20260614-154815-2133); the handler bodies are filled in by the
// per-worker taskets (state machine ff66, Gmail poll 5b07, OOO a81e, preflight
// d8f9). Until a handler is wired, its worker returns this error so the job
// stays retryable and visible in river rather than silently completing and
// dropping work.
var ErrHandlerNotWired = errors.New("sequences: job handler not wired")

// WorkspaceTxAcquirer is the river worker entry contract (ADR-004 rev 2 §3).
// The production implementation is *db.TenantPool (its AcquireTx method); a
// stub is substituted in tests. It is declared here, in the consumer, so the
// sequences package does not import the db package — the wiring layer
// (cmd/lecrm-api) injects the concrete *db.TenantPool.
type WorkspaceTxAcquirer interface {
	AcquireTx(ctx context.Context, workspaceID uuid.UUID) (context.Context, pgx.Tx, func(), error)
}

// Domain handler signatures. Each runs INSIDE the workspace-scoped tx that the
// worker has already opened via AcquireTx; it must not commit or roll back —
// the worker owns the tx lifecycle. Returning a non-nil error aborts the tx
// (the worker's deferred release rolls it back) and lets river retry the job
// per its MaxAttempts.
type (
	EnrollHandler    func(ctx context.Context, tx pgx.Tx, args EnrollArgs) error
	SendStepHandler  func(ctx context.Context, tx pgx.Tx, args SendStepArgs) error
	PollReplyHandler func(ctx context.Context, tx pgx.Tx, args PollReplyArgs) error
	FinalizeHandler  func(ctx context.Context, tx pgx.Tx, args FinalizeArgs) error

	// SendStepExhaustedHandler performs the send_step "final failure → failed"
	// transition (ADR-004 rev 2 §3). It runs in a FRESH tx after the failed
	// send tx has rolled back, and only on the final attempt. cause is the
	// error from that last send attempt.
	SendStepExhaustedHandler func(ctx context.Context, tx pgx.Tx, args SendStepArgs, cause error) error
)

// Handlers is the seam between the river framework (this tasket) and the
// domain logic (downstream taskets). A nil handler means "not wired yet": the
// matching worker returns ErrHandlerNotWired. Wiring is additive — each
// downstream tasket sets its own field without touching the others.
type Handlers struct {
	Enroll            EnrollHandler
	SendStep          SendStepHandler
	SendStepExhausted SendStepExhaustedHandler
	PollReply         PollReplyHandler
	Finalize          FinalizeHandler
}

// RegisterWorkers registers the four sequences workers on the river Workers
// bundle, each wired to acquire a workspace-scoped tx via acq and delegate to
// the matching handler in h. It returns the bundle for chaining.
//
// It uses river.AddWorker (which panics on a duplicate or invalid
// registration) deliberately: a misconfigured worker set is a programming
// error that should fail loudly at startup, not silently at job time. Because
// the four args types return four distinct Kind() values, a panic here would
// signal a real bug (e.g. a kind collision introduced by a future edit).
func RegisterWorkers(workers *river.Workers, acq WorkspaceTxAcquirer, h Handlers) *river.Workers {
	river.AddWorker(workers, &enrollWorker{acq: acq, handle: h.Enroll})
	river.AddWorker(workers, &sendStepWorker{acq: acq, handle: h.SendStep, onExhausted: h.SendStepExhausted})
	river.AddWorker(workers, &pollReplyWorker{acq: acq, handle: h.PollReply})
	river.AddWorker(workers, &finalizeWorker{acq: acq, handle: h.Finalize})
	return workers
}

// runInTx is the concrete AcquireTx contract (ADR-004 rev 2 §3): open a
// workspace-scoped tx via acq, run fn against it, and commit iff fn succeeds.
// The deferred release rolls the tx back on every non-commit path (error or
// panic) and returns the connection to the pool.
func runInTx(
	ctx context.Context,
	acq WorkspaceTxAcquirer,
	workspaceID uuid.UUID,
	fn func(ctx context.Context, tx pgx.Tx) error,
) error {
	ctx, tx, release, err := acq.AcquireTx(ctx, workspaceID)
	if err != nil {
		return err
	}
	defer release()

	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type enrollWorker struct {
	river.WorkerDefaults[EnrollArgs]
	acq    WorkspaceTxAcquirer
	handle EnrollHandler
}

func (w *enrollWorker) Work(ctx context.Context, job *river.Job[EnrollArgs]) error {
	return runInTx(ctx, w.acq, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
		if w.handle == nil {
			return ErrHandlerNotWired
		}
		return w.handle(ctx, tx, job.Args)
	})
}

type sendStepWorker struct {
	river.WorkerDefaults[SendStepArgs]
	acq         WorkspaceTxAcquirer
	handle      SendStepHandler
	onExhausted SendStepExhaustedHandler
}

func (w *sendStepWorker) Work(ctx context.Context, job *river.Job[SendStepArgs]) error {
	err := runInTx(ctx, w.acq, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
		if w.handle == nil {
			return ErrHandlerNotWired
		}
		return w.handle(ctx, tx, job.Args)
	})
	if err == nil {
		return nil
	}

	// Final failure → failed (ADR-004 rev 2 §3: send_step gets 5 attempts;
	// the final failure transitions the enrollment to `failed`). The send tx
	// has already rolled back, so the terminal transition runs in a fresh tx
	// and is durable even though the send itself failed. We still return the
	// original error so river records the job as discarded for observability.
	if job.Attempt >= job.MaxAttempts && w.onExhausted != nil {
		if ferr := runInTx(ctx, w.acq, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
			return w.onExhausted(ctx, tx, job.Args, err)
		}); ferr != nil {
			slog.ErrorContext(ctx, "sequences: send_step exhausted-transition failed",
				slog.String("enrollment_id", job.Args.EnrollmentID.String()),
				slog.Int("step_index", job.Args.StepIndex),
				slog.String("error", ferr.Error()),
			)
		}
	}
	return err
}

type pollReplyWorker struct {
	river.WorkerDefaults[PollReplyArgs]
	acq    WorkspaceTxAcquirer
	handle PollReplyHandler
}

func (w *pollReplyWorker) Work(ctx context.Context, job *river.Job[PollReplyArgs]) error {
	return runInTx(ctx, w.acq, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
		if w.handle == nil {
			return ErrHandlerNotWired
		}
		return w.handle(ctx, tx, job.Args)
	})
}

type finalizeWorker struct {
	river.WorkerDefaults[FinalizeArgs]
	acq    WorkspaceTxAcquirer
	handle FinalizeHandler
}

func (w *finalizeWorker) Work(ctx context.Context, job *river.Job[FinalizeArgs]) error {
	return runInTx(ctx, w.acq, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
		if w.handle == nil {
			return ErrHandlerNotWired
		}
		return w.handle(ctx, tx, job.Args)
	})
}

// Compile-time proof that each worker satisfies river.Worker for its args.
var (
	_ river.Worker[EnrollArgs]    = (*enrollWorker)(nil)
	_ river.Worker[SendStepArgs]  = (*sendStepWorker)(nil)
	_ river.Worker[PollReplyArgs] = (*pollReplyWorker)(nil)
	_ river.Worker[FinalizeArgs]  = (*finalizeWorker)(nil)
)
