package sequences

import (
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// TestArgs_KindsMatchConstants asserts each args type reports the JobKind
// constant it belongs to. The river adapter routes jobs by this string, so a
// drift between the args type and its kind would silently mis-route work.
func TestArgs_KindsMatchConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"enroll", EnrollArgs{}.Kind(), JobKindEnroll},
		{"send_step", SendStepArgs{}.Kind(), JobKindSendStep},
		{"poll_reply", PollReplyArgs{}.Kind(), JobKindPollReply},
		{"finalize", FinalizeArgs{}.Kind(), JobKindFinalize},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: Kind() = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestArgs_RetryBudgets pins the per-type MaxAttempts to the ADR-004 rev 2 §3
// "Retry policy" column: enroll 3, send_step 5, poll_reply 3, finalize 3.
func TestArgs_RetryBudgets(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"enroll", EnrollArgs{}.InsertOpts().MaxAttempts, 3},
		{"send_step", SendStepArgs{}.InsertOpts().MaxAttempts, 5},
		{"poll_reply", PollReplyArgs{}.InsertOpts().MaxAttempts, 3},
		{"finalize", FinalizeArgs{}.InsertOpts().MaxAttempts, 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: MaxAttempts = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestArgs_UniqueOpts pins the per-type river UniqueOpts to ADR-004 rev 2 §3:
// every type is ByArgs; finalize additionally sets ByState (one finalize per
// enrollment). The enroll/send_step/poll_reply types leave ByState unset
// (river applies its default unique-state set).
func TestArgs_UniqueOpts(t *testing.T) {
	if u := (EnrollArgs{}).InsertOpts().UniqueOpts; !u.ByArgs || u.ByState != nil {
		t.Errorf("enroll UniqueOpts = %+v, want {ByArgs:true, ByState:nil}", u)
	}
	if u := (SendStepArgs{}).InsertOpts().UniqueOpts; !u.ByArgs || u.ByState != nil {
		t.Errorf("send_step UniqueOpts = %+v, want {ByArgs:true, ByState:nil}", u)
	}
	if u := (PollReplyArgs{}).InsertOpts().UniqueOpts; !u.ByArgs || u.ByState != nil {
		t.Errorf("poll_reply UniqueOpts = %+v, want {ByArgs:true, ByState:nil}", u)
	}

	fu := (FinalizeArgs{}).InsertOpts().UniqueOpts
	if !fu.ByArgs {
		t.Errorf("finalize UniqueOpts.ByArgs = false, want true")
	}
	if len(fu.ByState) == 0 {
		t.Fatalf("finalize UniqueOpts.ByState is empty, want the default unique-state set")
	}
	// The finalize by-state set must be exactly river's default (it is what
	// gives "one finalize per enrollment" once paired with the ByArgs hash
	// scoped to enrollment_id).
	want := rivertype.UniqueOptsByStateDefault()
	if len(fu.ByState) != len(want) {
		t.Fatalf("finalize ByState = %v, want default %v", fu.ByState, want)
	}
	wantSet := map[rivertype.JobState]bool{}
	for _, s := range want {
		wantSet[s] = true
	}
	for _, s := range fu.ByState {
		if !wantSet[s] {
			t.Errorf("finalize ByState contains unexpected state %q", s)
		}
	}
}

// TestArgs_ImplementRiverInterfaces is a runtime echo of the compile-time
// assertions in args.go: each type is a usable river.JobArgs with insert opts.
func TestArgs_ImplementRiverInterfaces(t *testing.T) {
	var argsList = []river.JobArgs{
		EnrollArgs{},
		SendStepArgs{},
		PollReplyArgs{},
		FinalizeArgs{},
	}
	for _, a := range argsList {
		if a.Kind() == "" {
			t.Errorf("%T returned an empty Kind()", a)
		}
		if _, ok := a.(river.JobArgsWithInsertOpts); !ok {
			t.Errorf("%T does not implement river.JobArgsWithInsertOpts", a)
		}
	}
}
