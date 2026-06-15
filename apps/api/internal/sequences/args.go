package sequences

import (
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// Per-type retry budgets (ADR-004 rev 2 §3, "Retry policy" column). These
// are MaxAttempts: the original run plus all retries. river uses exponential
// backoff between attempts (its default policy); only the ceiling differs
// per job type.
const (
	maxAttemptsEnroll    = 3
	maxAttemptsSendStep  = 5 // final failure (attempt 5 errors) → enrollment transitions to `failed`
	maxAttemptsPollReply = 3
	maxAttemptsFinalize  = 3
)

// The four sequences river job arg types (ADR-004 rev 2 §3). Payloads carry
// IDs only — never PII — per ADR-009 §8.3: the worker re-fetches any record
// it needs through the workspace-scoped tx at execution time. Each type
// implements river.JobArgs (Kind) and river.JobArgsWithInsertOpts
// (InsertOpts), so its retry budget and uniqueness travel with the args and
// cannot drift from the registration site.
//
// WorkspaceID is on every payload because the worker's first action is
// AcquireTx(ctx, WorkspaceID) — the connection is opened as the workspace
// role before any data is touched. Within the per-tenant river_<hex> schema
// (ADR-009 §8.3) WorkspaceID is constant across all rows, so it is inert in
// the by-args uniqueness hash; it is carried for the AcquireTx call, not for
// uniqueness.

// EnrollArgs is the payload for sequences.enroll: enrol a contact into a
// sequence. UniqueOpts ByArgs prevents a double-enroll of the same
// (contact, sequence) while a prior enroll job is still in the queue.
type EnrollArgs struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ContactID   uuid.UUID `json:"contact_id"`
	SequenceID  uuid.UUID `json:"sequence_id"`
}

func (EnrollArgs) Kind() string { return JobKindEnroll }

func (EnrollArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttemptsEnroll,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// SendStepArgs is the payload for sequences.send_step: send one step of a
// sequence. The `river:"unique"` tags scope river's by-args uniqueness to
// (enrollment_id, step_index) exactly as ADR-004 rev 2 §3 specifies —
// IdempotencyKey is deliberately excluded from the queue-uniqueness hash so
// a supersede (which bumps the key, see SendStepIdempotencyKey) is not
// blocked as a "duplicate" by river. The DB-level partial unique index
// uniq_enrollment_step_active is the durable at-most-once backstop; river's
// UniqueOpts is the in-queue backstop (the Brandur belt-and-braces pair).
type SendStepArgs struct {
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	EnrollmentID   uuid.UUID `json:"enrollment_id" river:"unique"`
	StepIndex      int       `json:"step_index" river:"unique"`
	IdempotencyKey string    `json:"idempotency_key"`
}

func (SendStepArgs) Kind() string { return JobKindSendStep }

func (SendStepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttemptsSendStep,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// PollReplyArgs is the payload for sequences.poll_reply: check for and
// correlate a reply for an enrollment. IDs only — the inbound message body
// (PII) is fetched at execution time through the workspace tx, never carried
// on the payload. The Gmail-push / Brevo-inbound detail paths (taskets 5b07)
// extend the handler, not this payload.
type PollReplyArgs struct {
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	EnrollmentID uuid.UUID `json:"enrollment_id"`
}

func (PollReplyArgs) Kind() string { return JobKindPollReply }

func (PollReplyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttemptsPollReply,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// FinalizeArgs is the payload for sequences.finalize: record an enrollment
// reaching a terminal state. The `river:"unique"` tag on EnrollmentID alone
// (TerminalState and Reason excluded) plus ByState gives the ADR-004 rev 2
// §3 guarantee of "one finalize per enrollment": once a finalize for an
// enrollment exists in any of the default unique states, a second cannot be
// inserted, regardless of the terminal_state or reason it carries.
type FinalizeArgs struct {
	WorkspaceID   uuid.UUID `json:"workspace_id"`
	EnrollmentID  uuid.UUID `json:"enrollment_id" river:"unique"`
	TerminalState string    `json:"terminal_state"`
	Reason        string    `json:"reason"`
}

func (FinalizeArgs) Kind() string { return JobKindFinalize }

func (FinalizeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: maxAttemptsFinalize,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByState: rivertype.UniqueOptsByStateDefault(),
		},
	}
}

// Compile-time proof that each args type satisfies both river interfaces.
var (
	_ river.JobArgs               = EnrollArgs{}
	_ river.JobArgs               = SendStepArgs{}
	_ river.JobArgs               = PollReplyArgs{}
	_ river.JobArgs               = FinalizeArgs{}
	_ river.JobArgsWithInsertOpts = EnrollArgs{}
	_ river.JobArgsWithInsertOpts = SendStepArgs{}
	_ river.JobArgsWithInsertOpts = PollReplyArgs{}
	_ river.JobArgsWithInsertOpts = FinalizeArgs{}
)
