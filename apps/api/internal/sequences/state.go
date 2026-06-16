package sequences

import "sort"

// State is one enrollment lifecycle state. The string values mirror the
// Postgres `enrollment_state` ENUM defined in ADR-004 rev 2 §1 exactly,
// so a State round-trips to and from the column without a translation
// layer. Do not rename a value without an accompanying migration.
type State string

// Enrollment states (ADR-004 rev 2 §1–§2). The six-state framing in the
// tasket (enrolled → step_sent → waiting_reply → reply_received /
// ooo_detected / failed) maps cleanly onto this richer set; the four
// extra terminal states (bounced, unsubscribed, suppressed, completed)
// carry distinct retention and reporting semantics per §2.
const (
	StateEnrolled      State = "enrolled"       // entry; a send_step job is scheduled at next_action_at
	StateStepSent      State = "step_sent"      // a step was sent; awaiting reply-window events
	StateWaitingReply  State = "waiting_reply"  // inside the reply window; poll_reply active
	StateReplyReceived State = "reply_received" // reply matched & classified non-OOO (terminal* in v1)
	StateOOODetected   State = "ooo_detected"   // reply classified out-of-office; resumes at ooo_returns_at
	StateFailed        State = "failed"         // provider 5xx after max retries (terminal)
	StateBounced       State = "bounced"        // hard bounce (terminal)
	StateUnsubscribed  State = "unsubscribed"   // List-Unsubscribe / complaint (terminal)
	StateSuppressed    State = "suppressed"     // suppression hit pre-send (terminal)
	StateCompleted     State = "completed"      // all steps sent, no reply (terminal)
)

// allStates is every declared State, in lifecycle-ish order. Used by
// Valid and exercised by the table-invariant tests.
var allStates = []State{
	StateEnrolled,
	StateStepSent,
	StateWaitingReply,
	StateReplyReceived,
	StateOOODetected,
	StateFailed,
	StateBounced,
	StateUnsubscribed,
	StateSuppressed,
	StateCompleted,
}

// terminalStates are the states with no outgoing transitions. Reaching
// one of these is what fires the sequences.finalize job (ADR-004 rev 2
// §3). reply_received is terminal for the enrollment in v1; v2 reply
// branching attaches here without a schema change (§2).
var terminalStates = map[State]struct{}{
	StateReplyReceived: {},
	StateFailed:        {},
	StateBounced:       {},
	StateUnsubscribed:  {},
	StateSuppressed:    {},
	StateCompleted:     {},
}

// legalTransitions is the legal `from → set(to)` graph, transcribed from
// the ADR-004 rev 2 §2 state diagram (cross-referenced with §4 reply
// correlation, §5 OOO resume, and §8 suppression/bounce policy). It is
// the single source of truth consumed by the state-machine tasket's
// Transition() (20260614-154815-ff66); this package only reads it.
//
// The send_step worker runs while the enrollment is in StateEnrolled
// (the scheduled next action), so its three outcomes — sent, suppressed
// pre-send, or send-failed-after-retries — are edges out of enrolled.
// Async post-send events (bounce, unsubscribe, reply) are edges out of
// step_sent / waiting_reply.
//
// Edge-case edges not literally drawn in the §2 diagram but required by
// the surrounding prose (e.g. a late bounce arriving during the reply
// window) are intentionally left for the Transition() tasket to confirm
// against live provider behavior; this table encodes only edges with a
// direct ADR citation, noted per edge below.
var legalTransitions = map[State][]State{
	StateEnrolled: {
		StateStepSent,   // §2: enrolled --(send_step job)--> step_sent
		StateSuppressed, // §2/§8: suppression hit pre-send
		StateFailed,     // §2/§3: provider 5xx, max retries before any send succeeds
	},
	StateStepSent: {
		StateWaitingReply, // §2/§4: enter the reply window; poll_reply scheduled
		StateBounced,      // §2/§8: hard bounce
		StateUnsubscribed, // §2/§8: List-Unsubscribe / complaint
		StateFailed,       // §2/§3: provider 5xx, max retries
		StateCompleted,    // §2: last step, no reply window → done
	},
	StateWaitingReply: {
		StateReplyReceived, // §4: reply matched, classified non-OOO
		StateOOODetected,   // §4/§5: reply matched, classified OOO
		StateEnrolled,      // §2: reply window expired, more steps remain → next step
		StateCompleted,     // §2: reply window expired, last step → done
	},
	StateOOODetected: {
		StateEnrolled,  // §2/§5: ooo_returns_at reached → resume at next step
		StateCompleted, // §5: OOO was the last step → done
	},
	// Terminal states have no outgoing edges and are absent from this map.
}

// Valid reports whether s is a declared enrollment State.
func (s State) Valid() bool {
	for _, k := range allStates {
		if s == k {
			return true
		}
	}
	return false
}

// IsTerminal reports whether s is a terminal state (no outgoing
// transitions; entering it fires sequences.finalize).
func (s State) IsTerminal() bool {
	_, ok := terminalStates[s]
	return ok
}

// CanTransition reports whether from → to is a legal transition per the
// ADR-004 rev 2 §2 table. It is a pure predicate over static data: it
// performs no I/O and does not mutate anything. The state-machine
// tasket's Transition() calls this for validation before it locks the
// row and writes; an invalid transition is a programming error (§2:
// panic in dev/test, sequences.transition.invalid audit row in prod).
func CanTransition(from, to State) bool {
	for _, allowed := range legalTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// AllowedTransitions returns a sorted copy of the states reachable from
// `from` in one transition. Returns an empty slice for terminal or
// unknown states. The returned slice is a copy; callers may mutate it
// without affecting the table.
func AllowedTransitions(from State) []State {
	src := legalTransitions[from]
	out := make([]State, len(src))
	copy(out, src)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// AllStates returns a copy of every declared State, in lifecycle order.
func AllStates() []State {
	out := make([]State, len(allStates))
	copy(out, allStates)
	return out
}
