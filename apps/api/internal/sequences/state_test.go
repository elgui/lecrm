package sequences

import (
	"sort"
	"testing"
)

// expectedStateValues mirrors the Postgres `enrollment_state` ENUM in
// ADR-004 rev 2 §1 exactly. If this set drifts from state.go, the State
// constants no longer round-trip to the column — which a migration must
// catch, not a silent runtime cast.
var expectedStateValues = map[State]struct{}{
	"enrolled": {}, "step_sent": {}, "waiting_reply": {}, "reply_received": {},
	"ooo_detected": {}, "failed": {}, "bounced": {}, "unsubscribed": {},
	"suppressed": {}, "completed": {},
}

func TestStateValuesMatchPostgresEnum(t *testing.T) {
	if len(allStates) != len(expectedStateValues) {
		t.Fatalf("allStates has %d entries, ENUM has %d", len(allStates), len(expectedStateValues))
	}
	seen := map[State]bool{}
	for _, s := range allStates {
		if _, ok := expectedStateValues[s]; !ok {
			t.Errorf("state %q is not in the ADR-004 rev 2 §1 enrollment_state ENUM", s)
		}
		if seen[s] {
			t.Errorf("state %q declared twice in allStates", s)
		}
		seen[s] = true
	}
}

func TestValid(t *testing.T) {
	for _, s := range allStates {
		if !s.Valid() {
			t.Errorf("Valid(%q) = false, want true", s)
		}
	}
	for _, bad := range []State{"", "ENROLLED", "sent", "unknown"} {
		if bad.Valid() {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
}

// TestTerminalPartition: terminal and non-terminal states partition the
// full state set (cover everything, overlap nothing).
func TestTerminalPartition(t *testing.T) {
	for s := range terminalStates {
		if !s.Valid() {
			t.Errorf("terminal state %q is not a declared State", s)
		}
		if !s.IsTerminal() {
			t.Errorf("IsTerminal(%q) = false, want true", s)
		}
	}
	wantTerminal := map[State]bool{
		StateReplyReceived: true, StateFailed: true, StateBounced: true,
		StateUnsubscribed: true, StateSuppressed: true, StateCompleted: true,
	}
	for _, s := range allStates {
		if got := s.IsTerminal(); got != wantTerminal[s] {
			t.Errorf("IsTerminal(%q) = %v, want %v", s, got, wantTerminal[s])
		}
	}
}

// TestTerminalStatesHaveNoOutgoingEdges: a terminal state must not appear
// as a source in the transition table. Entering it ends the lifecycle
// (fires sequences.finalize).
func TestTerminalStatesHaveNoOutgoingEdges(t *testing.T) {
	for s := range terminalStates {
		if outs, ok := legalTransitions[s]; ok && len(outs) > 0 {
			t.Errorf("terminal state %q has outgoing transitions %v", s, outs)
		}
		if got := AllowedTransitions(s); len(got) != 0 {
			t.Errorf("AllowedTransitions(%q) = %v, want empty", s, got)
		}
	}
}

// TestNonTerminalStatesHaveOutgoingEdges: every non-terminal state must
// be able to leave — otherwise an enrollment could wedge forever in a
// non-terminal state with no legal exit.
func TestNonTerminalStatesHaveOutgoingEdges(t *testing.T) {
	for _, s := range allStates {
		if s.IsTerminal() {
			continue
		}
		if len(legalTransitions[s]) == 0 {
			t.Errorf("non-terminal state %q has no outgoing transitions (would wedge)", s)
		}
	}
}

// TestTransitionTableEndpointsAreValidStates: every source and target in
// the table is a declared State (guards typos), and no source is terminal.
func TestTransitionTableEndpointsAreValidStates(t *testing.T) {
	for from, tos := range legalTransitions {
		if !from.Valid() {
			t.Errorf("transition source %q is not a declared State", from)
		}
		if from.IsTerminal() {
			t.Errorf("terminal state %q is used as a transition source", from)
		}
		seen := map[State]bool{}
		for _, to := range tos {
			if !to.Valid() {
				t.Errorf("transition %q → %q targets an undeclared State", from, to)
			}
			if to == from {
				t.Errorf("self-loop transition %q → %q", from, to)
			}
			if seen[to] {
				t.Errorf("duplicate transition %q → %q", from, to)
			}
			seen[to] = true
		}
	}
}

func TestCanTransitionHappyPath(t *testing.T) {
	// ADR-004 rev 2 §2 primary path.
	legal := [][2]State{
		{StateEnrolled, StateStepSent},
		{StateStepSent, StateWaitingReply},
		{StateWaitingReply, StateReplyReceived},
		// OOO branch + resume (§4/§5).
		{StateWaitingReply, StateOOODetected},
		{StateOOODetected, StateEnrolled},
		// window-expiry advance to next step (§2).
		{StateWaitingReply, StateEnrolled},
		// pre-send suppression (§8) and send failure (§3).
		{StateEnrolled, StateSuppressed},
		{StateEnrolled, StateFailed},
		// async post-send terminals (§8).
		{StateStepSent, StateBounced},
		{StateStepSent, StateUnsubscribed},
	}
	for _, tc := range legal {
		if !CanTransition(tc[0], tc[1]) {
			t.Errorf("CanTransition(%q, %q) = false, want true", tc[0], tc[1])
		}
	}
}

func TestCanTransitionRejectsIllegal(t *testing.T) {
	illegal := [][2]State{
		{StateEnrolled, StateReplyReceived}, // can't reply before a send
		{StateEnrolled, StateWaitingReply},  // must pass through step_sent
		{StateStepSent, StateEnrolled},      // no backward jump without a reply window
		{StateReplyReceived, StateEnrolled}, // terminal has no exits
		{StateCompleted, StateStepSent},     // terminal has no exits
		{StateSuppressed, StateEnrolled},    // terminal has no exits
		{"bogus", StateStepSent},            // unknown source
		{StateEnrolled, "bogus"},            // unknown target
	}
	for _, tc := range illegal {
		if CanTransition(tc[0], tc[1]) {
			t.Errorf("CanTransition(%q, %q) = true, want false", tc[0], tc[1])
		}
	}
}

// TestEveryStateReachesATerminal: from any state, a BFS over the legal
// transitions must reach some terminal state. This proves the machine
// always has a path to completion — no non-terminal sink.
func TestEveryStateReachesATerminal(t *testing.T) {
	for _, start := range allStates {
		if !reachesTerminal(start) {
			t.Errorf("state %q has no path to any terminal state", start)
		}
	}
}

// TestAllStatesReachableFromEnrolled: BFS from the entry state must visit
// every declared state — guards against a typo orphaning a state out of
// the graph.
func TestAllStatesReachableFromEnrolled(t *testing.T) {
	visited := map[State]bool{StateEnrolled: true}
	queue := []State{StateEnrolled}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, to := range legalTransitions[cur] {
			if !visited[to] {
				visited[to] = true
				queue = append(queue, to)
			}
		}
	}
	for _, s := range allStates {
		if !visited[s] {
			t.Errorf("state %q is unreachable from StateEnrolled", s)
		}
	}
}

func reachesTerminal(start State) bool {
	visited := map[State]bool{start: true}
	queue := []State{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.IsTerminal() {
			return true
		}
		for _, to := range legalTransitions[cur] {
			if !visited[to] {
				visited[to] = true
				queue = append(queue, to)
			}
		}
	}
	return false
}

func TestAllowedTransitionsIsSortedDefensiveCopy(t *testing.T) {
	got := AllowedTransitions(StateStepSent)
	if len(got) == 0 {
		t.Fatal("AllowedTransitions(StateStepSent) is empty")
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Errorf("AllowedTransitions not sorted: %v", got)
	}
	// Mutating the returned slice must not corrupt the table.
	got[0] = "MUTATED"
	again := AllowedTransitions(StateStepSent)
	for _, s := range again {
		if s == "MUTATED" {
			t.Fatal("AllowedTransitions returned a slice aliasing the internal table")
		}
	}
}

func TestAllStatesIsDefensiveCopy(t *testing.T) {
	out := AllStates()
	if len(out) != len(allStates) {
		t.Fatalf("AllStates len = %d, want %d", len(out), len(allStates))
	}
	out[0] = "MUTATED"
	if allStates[0] == "MUTATED" {
		t.Fatal("AllStates returned a slice aliasing the internal allStates")
	}
}
