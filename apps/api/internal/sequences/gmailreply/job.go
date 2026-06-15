package gmailreply

import (
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// Gmail-path river job kinds. They live under the sequences.gmail.* namespace —
// distinct from the foundation's four sequences.* kinds (sequences/jobs.go) —
// so this path can ship a mailbox-scoped scan and a watch-renewal job without
// touching the foundation's job set or the per-enrollment PollReplyArgs payload.
const (
	// JobKindPollMailbox scans one mailbox's Gmail history since the persisted
	// cursor and correlates replies. Enqueued by the push handler on every
	// Pub/Sub delivery. Args: PollMailboxArgs.
	JobKindPollMailbox = "sequences.gmail.poll_mailbox"

	// JobKindWatchRenew re-registers a workspace's Gmail watch. Inserted by the
	// PeriodicWatchRenew job (cron "0 4 * * *"). Args: WatchRenewArgs.
	JobKindWatchRenew = "sequences.gmail.watch_renew"
)

// Per-type retry budgets. Both jobs are safe to retry: poll_mailbox is
// idempotent (it re-scans from the persisted cursor and Transition is
// at-most-once per enrollment), and watch_renew just re-issues users.watch().
const (
	maxAttemptsPollMailbox = 3
	maxAttemptsWatchRenew  = 3
)

// PollMailboxArgs is the payload for sequences.gmail.poll_mailbox. It is keyed
// on the mailbox (workspace + user), NOT an enrollment: one push triggers one
// history scan that may correlate replies across many enrollments. The
// river:"unique" tags scope by-args uniqueness to (workspace_id, user_id) so a
// burst of pushes for the same mailbox coalesces into a single queued scan
// (EmailAddress and HistoryID are deliberately excluded from the hash).
//
// HistoryID is the notification's advisory cursor; the worker scans from the
// *persisted* connection cursor (which may be older), so coalescing pushes
// never skips events.
type PollMailboxArgs struct {
	WorkspaceID  uuid.UUID `json:"workspace_id" river:"unique"`
	UserID       uuid.UUID `json:"user_id" river:"unique"`
	EmailAddress string    `json:"email_address"`
	HistoryID    uint64    `json:"history_id"`
}

// Kind implements river.JobArgs.
func (PollMailboxArgs) Kind() string { return JobKindPollMailbox }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (PollMailboxArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttemptsPollMailbox,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// WatchRenewArgs is the payload for sequences.gmail.watch_renew. It carries the
// workspace id because the periodic job is registered per-workspace (each
// workspace has its own river client + schema); the worker uses it for the
// AcquireTx scope, exactly like the foundation's args.
type WatchRenewArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
}

// Kind implements river.JobArgs.
func (WatchRenewArgs) Kind() string { return JobKindWatchRenew }

// InsertOpts implements river.JobArgsWithInsertOpts. ByPeriod collapses
// duplicate inserts if a leader re-election re-fires the schedule within the
// window — only one watch_renew per workspace per hour can be queued.
func (WatchRenewArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttemptsWatchRenew,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByPeriod: time.Hour},
	}
}

// watchRenewHour is the hour-of-day (process local time, UTC in production) the
// daily watch-renewal runs — the "4" in cron "0 4 * * *".
const watchRenewHour = 4

// dailySchedule fires once per calendar day at hour:00 in the supplied time's
// location. It is the river.PeriodicSchedule equivalent of cron "0 <hour> * * *"
// without pulling in a cron dependency for a single fixed daily tick.
type dailySchedule struct{ hour int }

// Next returns the next hour:00 strictly after current.
func (s dailySchedule) Next(current time.Time) time.Time {
	next := time.Date(current.Year(), current.Month(), current.Day(), s.hour, 0, 0, 0, current.Location())
	if !next.After(current) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// WatchRenewSchedule is the daily-at-04:00 schedule (cron "0 4 * * *", ADR-004
// rev 2 §4) for the Gmail watch-renewal periodic job.
func WatchRenewSchedule() river.PeriodicSchedule { return dailySchedule{hour: watchRenewHour} }

// PeriodicWatchRenew builds the river periodic job that renews workspaceID's
// Gmail watch every day at 04:00. RunOnStart hedges a long-running leaderless
// gap (a watch could otherwise lapse before the first 04:00 after a restart).
// The wiring layer adds the returned job to its workspace river.Config.PeriodicJobs.
func PeriodicWatchRenew(workspaceID uuid.UUID) *river.PeriodicJob {
	return river.NewPeriodicJob(
		WatchRenewSchedule(),
		func() (river.JobArgs, *river.InsertOpts) {
			return WatchRenewArgs{WorkspaceID: workspaceID}, nil
		},
		&river.PeriodicJobOpts{ID: JobKindWatchRenew, RunOnStart: true},
	)
}

// Compile-time proof both args types satisfy the river interfaces.
var (
	_ river.JobArgs               = PollMailboxArgs{}
	_ river.JobArgs               = WatchRenewArgs{}
	_ river.JobArgsWithInsertOpts = PollMailboxArgs{}
	_ river.JobArgsWithInsertOpts = WatchRenewArgs{}
	_ river.PeriodicSchedule      = dailySchedule{}
)
