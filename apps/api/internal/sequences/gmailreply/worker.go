package gmailreply

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

// detectorGmailPush is the audit-payload `detector` value for replies found via
// the Gmail push path (ADR-004 rev 2 §6 sequences.reply_received fields).
const detectorGmailPush = "gmail_push"

// callerPollMailbox identifies this worker in sequences.transition.invalid audit
// rows (WithCaller) so a stray illegal transition is traceable here.
const callerPollMailbox = "gmail.poll_mailbox"

// transitionFunc is the enrollment state-machine write path. It is the seam by
// which the worker calls sequences.Transition; tests inject a recorder so the
// correlation/classification orchestration is verifiable without a database.
type transitionFunc func(ctx context.Context, tx pgx.Tx, enrollmentID uuid.UUID, to sequences.State, reason string, opts ...sequences.Option) error

// Deps bundles the collaborators the Gmail-path workers need. The exported
// fields are set by the wiring layer; the unexported seams default in
// resolved() and are overridden by same-package tests.
type Deps struct {
	// Acquirer opens the workspace-scoped tx each worker runs in.
	Acquirer sequences.WorkspaceTxAcquirer
	// Clients builds a mailbox HistoryClient for (workspace, user).
	Clients ClientFactory
	// Classifier decides reply_received vs ooo_detected (defaults to
	// DefaultClassifier until order:7 wires the real one).
	Classifier Classifier
	// Finalize enqueues sequences.finalize when a reply transition is terminal
	// (reply_received). Optional; nil skips the enqueue (the finalize still
	// happens via the timeout path).
	Finalize sequences.FinalizeEnqueuer
	// Logger for structured progress/diagnostic logs. Defaults to slog.Default().
	Logger *slog.Logger

	transition transitionFunc
	now        func() time.Time
}

// resolved returns a copy of d with the test seams and optional collaborators
// defaulted.
func (d Deps) resolved() Deps {
	if d.transition == nil {
		d.transition = sequences.Transition
	}
	if d.now == nil {
		d.now = time.Now
	}
	if d.Classifier == nil {
		d.Classifier = DefaultClassifier{}
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	return d
}

// RegisterWorkers registers the two Gmail-path workers (poll_mailbox,
// watch_renew) on the river Workers bundle. It is additive to the foundation's
// sequences.RegisterWorkers — the wiring layer calls both on the same bundle.
// Like the foundation, it uses river.AddWorker (which panics on a duplicate or
// invalid registration) so a misconfigured worker set fails loudly at startup.
func RegisterWorkers(workers *river.Workers, deps Deps) *river.Workers {
	d := deps.resolved()
	river.AddWorker(workers, &pollMailboxWorker{deps: d})
	river.AddWorker(workers, &watchRenewWorker{deps: d})
	return workers
}

// runInTx opens a workspace-scoped tx via acq, runs fn, and commits iff fn
// succeeds; the deferred release rolls back on every non-commit path. It mirrors
// the foundation's unexported sequences.runInTx envelope (re-stated here so this
// package does not need the db package and the foundation needs no new export).
func runInTx(
	ctx context.Context,
	acq sequences.WorkspaceTxAcquirer,
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

// --- poll_mailbox ---

type pollMailboxWorker struct {
	river.WorkerDefaults[PollMailboxArgs]
	deps Deps
}

func (w *pollMailboxWorker) Work(ctx context.Context, job *river.Job[PollMailboxArgs]) error {
	return runInTx(ctx, w.deps.Acquirer, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
		return w.deps.pollMailbox(ctx, tx, job.Args)
	})
}

// pollMailbox is the reply-correlation scan (ADR-004 rev 2 §4 steps 1–4). It
// runs inside the workspace tx: load the cursor, scan Gmail history once,
// batch-match referenced Message-IDs against sent steps, and transition each
// matched enrollment. It is fail-closed — any error rolls the whole tx back
// (state changes AND cursor advance) so river retries from a consistent point.
func (d Deps) pollMailbox(ctx context.Context, tx pgx.Tx, args PollMailboxArgs) error {
	cli, err := d.Clients.Client(ctx, args.WorkspaceID, args.UserID)
	if err != nil {
		return fmt.Errorf("gmailreply: build client: %w", err)
	}

	start, found, err := loadCursor(ctx, tx)
	if err != nil {
		return err
	}
	if !found {
		// No persisted cursor yet. Baseline from the notification's historyId;
		// if even that is absent, establish a baseline via watch and stop —
		// there is no scannable window before the first cursor.
		start = args.HistoryID
		if start == 0 {
			hid, _, werr := cli.Watch(ctx)
			if werr != nil {
				return fmt.Errorf("gmailreply: baseline watch: %w", werr)
			}
			return saveCursor(ctx, tx, hid)
		}
	}

	msgs, newHID, err := cli.MessagesSince(ctx, start)
	if errors.Is(err, ErrHistoryGap) {
		return d.rebaseline(ctx, tx, cli, args)
	}
	if err != nil {
		return fmt.Errorf("gmailreply: history scan: %w", err)
	}

	if len(msgs) > 0 {
		matched, err := matchSteps(ctx, tx, collectReferencedIDs(msgs))
		if err != nil {
			return err
		}
		for _, m := range correlateReplies(msgs, matched) {
			if err := d.applyReply(ctx, tx, m); err != nil {
				// An illegal transition here is a benign race, not a batch failure:
				// the enrollment already left waiting_reply (e.g. a redelivered
				// Pub/Sub push another worker already correlated, or a reply matched
				// after the window closed). matchSteps' state read is stale relative
				// to Transition's SELECT … FOR UPDATE, so the loser sees an illegal
				// edge. InvalidAudit has already written the sequences.transition.invalid
				// trace on this tx; skip THIS reply and keep the rest of the batch —
				// and the cursor advance — intact, instead of rolling the whole
				// scanned window back and reprocessing it. Any other error is
				// transient/real and must abort so no reply is skipped past the cursor.
				if errors.Is(err, sequences.ErrInvalidTransition) {
					d.Logger.WarnContext(ctx, "gmail reply skipped: illegal transition (already correlated?)",
						"enrollment_id", m.EnrollmentID.String(),
						"error", err.Error(),
					)
					continue
				}
				return err
			}
		}
	}

	// Advance the cursor so the scanned window is never re-processed. Only move
	// forward; a stale/lower newHID never rewinds the cursor.
	if newHID > start {
		return saveCursor(ctx, tx, newHID)
	}
	return nil
}

// rebaseline recovers from an ErrHistoryGap (cursor older than Gmail retains).
// It re-arms the watch and adopts its current historyId as the new cursor. The
// gap window cannot be reconstructed; future events resume cleanly. (ADR-004
// rev 2 §4: "handle history-gap (full re-sync) safely".)
func (d Deps) rebaseline(ctx context.Context, tx pgx.Tx, cli HistoryClient, args PollMailboxArgs) error {
	d.Logger.WarnContext(ctx, "gmail history gap; re-baselining cursor",
		"workspace_id", args.WorkspaceID.String(), "email", args.EmailAddress)
	hid, _, err := cli.Watch(ctx)
	if err != nil {
		return fmt.Errorf("gmailreply: rebaseline watch: %w", err)
	}
	return saveCursor(ctx, tx, hid)
}

// applyReply classifies one correlated reply and writes the implied transition
// in the worker's tx. reply_received is terminal (v1) and enqueues finalize;
// ooo_detected sets ooo_returns_at / next_action_at so the enrollment resumes.
func (d Deps) applyReply(ctx context.Context, tx pgx.Tx, m ReplyMatch) error {
	cls, err := d.Classifier.Classify(ctx, m.Message)
	if err != nil {
		return fmt.Errorf("gmailreply: classify reply for %s: %w", m.EnrollmentID, err)
	}

	payload := map[string]any{
		"step_index":            m.StepIndex,
		"reply_message_id":      m.ReplyMessageID,
		"classifier_category":   cls.Category,
		"classifier_confidence": cls.Confidence,
		"detector":              detectorGmailPush,
	}
	opts := []sequences.Option{
		sequences.WithReplyMessageID(m.ReplyMessageID),
		sequences.WithInvalidMode(sequences.InvalidAudit),
		sequences.WithCaller(callerPollMailbox),
		sequences.WithPayload(payload),
	}

	to := sequences.StateReplyReceived
	reason := "gmail reply detected"
	if cls.IsOOO {
		to = sequences.StateOOODetected
		reason = "gmail out-of-office detected"
		if cls.OOOReturnsAt != nil {
			opts = append(opts,
				sequences.WithOOOReturnsAt(*cls.OOOReturnsAt),
				sequences.WithNextActionAt(*cls.OOOReturnsAt),
			)
		}
	} else if d.Finalize != nil {
		// reply_received is terminal in v1 → enqueue sequences.finalize on this tx.
		opts = append(opts, sequences.WithFinalizeEnqueuer(d.Finalize))
	}

	if err := d.transition(ctx, tx, m.EnrollmentID, to, reason, opts...); err != nil {
		return fmt.Errorf("gmailreply: transition %s → %s: %w", m.EnrollmentID, to, err)
	}
	d.Logger.InfoContext(ctx, "gmail reply correlated",
		"enrollment_id", m.EnrollmentID.String(),
		"to", string(to),
		"ooo", cls.IsOOO,
		"category", cls.Category,
	)
	return nil
}

// --- watch_renew ---

type watchRenewWorker struct {
	river.WorkerDefaults[WatchRenewArgs]
	deps Deps
}

func (w *watchRenewWorker) Work(ctx context.Context, job *river.Job[WatchRenewArgs]) error {
	return runInTx(ctx, w.deps.Acquirer, job.Args.WorkspaceID, func(ctx context.Context, tx pgx.Tx) error {
		return w.deps.watchRenew(ctx, tx, job.Args)
	})
}

// watchRenew re-registers users.watch() for each active Gmail connection so
// Gmail keeps publishing notifications (a watch expires after 7 days; ADR-004
// rev 2 §4 renews daily). Re-arming the watch does NOT advance the scan cursor —
// the cursor tracks scan progress independently and must keep its position so no
// unscanned history is skipped; the watch's historyId is adopted only as the
// first-time baseline. One connection failing does not abort the others.
func (d Deps) watchRenew(ctx context.Context, tx pgx.Tx, args WatchRenewArgs) error {
	conns, err := listActiveGmailConnections(ctx, tx)
	if err != nil {
		return err
	}
	if len(conns) == 0 {
		d.Logger.InfoContext(ctx, "gmail watch_renew: no active connections",
			"workspace_id", args.WorkspaceID.String())
		return nil
	}

	var firstErr error
	for _, conn := range conns {
		cli, err := d.Clients.Client(ctx, args.WorkspaceID, conn.UserID)
		if err != nil {
			d.Logger.ErrorContext(ctx, "gmail watch_renew: build client failed",
				"workspace_id", args.WorkspaceID.String(), "user_id", conn.UserID.String(), "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		hid, exp, err := cli.Watch(ctx)
		if err != nil {
			d.Logger.ErrorContext(ctx, "gmail watch_renew: watch failed",
				"workspace_id", args.WorkspaceID.String(), "user_id", conn.UserID.String(), "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Adopt the watch historyId only when no cursor exists yet; otherwise
		// keep the scan cursor where it is.
		if conn.HistoryID == 0 {
			if err := saveCursor(ctx, tx, hid); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
		}
		d.Logger.InfoContext(ctx, "gmail watch renewed",
			"workspace_id", args.WorkspaceID.String(),
			"user_id", conn.UserID.String(),
			"expiry", exp.Format(time.RFC3339),
		)
	}
	// Returning the first error makes river retry the renewal (idempotent), so a
	// transient Google blip does not silently leave a watch un-renewed.
	return firstErr
}

// Compile-time proof each worker satisfies river.Worker for its args.
var (
	_ river.Worker[PollMailboxArgs] = (*pollMailboxWorker)(nil)
	_ river.Worker[WatchRenewArgs]  = (*watchRenewWorker)(nil)
)
