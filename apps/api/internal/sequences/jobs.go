package sequences

// River job kinds for the sequences engine (ADR-004 rev 2 §3). Four
// types, all tenant-scoped per ADR-009 §8.3: the worker acquires a
// workspace-scoped connection (jobs.RunWorkspaceJob) before any data
// operation, and the river table lives in the per-workspace
// river_<workspace_base36> schema.
//
// Naming convention is dot-separated and mirrors the audit-log event
// names (see audit.go and the email package's JobKind* constants), so a
// job kind and its emitted audit event read the same in logs.
//
// These are the shared kind strings only. The args structs, river
// UniqueOpts wiring, retry policies, and handler bodies belong to the
// river-framework tasket (20260614-154815-2133) and the per-worker
// taskets; they import these constants rather than redeclaring the
// strings.
const (
	// JobKindEnroll enrolls a contact into a sequence.
	// Args: {contact_id, sequence_id}. river UniqueOpts: by-args
	// (prevents double-enroll). Retry: exp backoff, 3 attempts.
	JobKindEnroll = "sequences.enroll"

	// JobKindSendStep sends one sequence step. Args:
	// {enrollment_id, step_index, idempotency_key}. Scheduled at
	// enrollments.next_action_at. river UniqueOpts: by-args on
	// (enrollment_id, step_index) — paired with the partial unique index
	// uniq_enrollment_step_active (§1) for belt-and-braces at-most-once.
	// Retry: exp backoff, 5 attempts; final failure → StateFailed.
	JobKindSendStep = "sequences.send_step"

	// JobKindPollReply checks for and correlates a reply. Args:
	// {enrollment_id}. Enqueued on entering StateWaitingReply and
	// re-checked periodically until the reply window expires. river
	// UniqueOpts: by-args. Retry: exp backoff, 3 attempts.
	JobKindPollReply = "sequences.poll_reply"

	// JobKindFinalize records an enrollment reaching a terminal state.
	// Args: {enrollment_id, terminal_state, reason}. Enqueued by
	// Transition() when entering a terminal state. river UniqueOpts:
	// by-args + by-state (one finalize per enrollment). Retry: exp
	// backoff, 3 attempts.
	JobKindFinalize = "sequences.finalize"
)

// allJobKinds is every declared sequences job kind.
var allJobKinds = []string{
	JobKindEnroll,
	JobKindSendStep,
	JobKindPollReply,
	JobKindFinalize,
}

// AllJobKinds returns a copy of the four sequences river job kinds. The
// river-framework tasket uses it to register workers and to assert the
// registered set matches the declared set.
func AllJobKinds() []string {
	out := make([]string, len(allJobKinds))
	copy(out, allJobKinds)
	return out
}
