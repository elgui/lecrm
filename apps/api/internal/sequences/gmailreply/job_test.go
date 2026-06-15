package gmailreply

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

func TestPollMailboxArgs_KindAndOpts(t *testing.T) {
	var a PollMailboxArgs
	if a.Kind() != "sequences.gmail.poll_mailbox" {
		t.Errorf("Kind = %q", a.Kind())
	}
	o := a.InsertOpts()
	if o.MaxAttempts != maxAttemptsPollMailbox {
		t.Errorf("MaxAttempts = %d, want %d", o.MaxAttempts, maxAttemptsPollMailbox)
	}
	if !o.UniqueOpts.ByArgs {
		t.Error("poll_mailbox should be unique ByArgs to coalesce push bursts")
	}
}

func TestWatchRenewArgs_KindAndOpts(t *testing.T) {
	var a WatchRenewArgs
	if a.Kind() != "sequences.gmail.watch_renew" {
		t.Errorf("Kind = %q", a.Kind())
	}
	o := a.InsertOpts()
	if o.UniqueOpts.ByPeriod == 0 {
		t.Error("watch_renew should be unique ByPeriod to dedupe leader re-fires")
	}
}

func TestDailySchedule_Next(t *testing.T) {
	loc := time.UTC
	s := dailySchedule{hour: 4}

	// Before 04:00 → same day 04:00.
	cur := time.Date(2026, 6, 15, 1, 30, 0, 0, loc)
	got := s.Next(cur)
	want := time.Date(2026, 6, 15, 4, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(01:30) = %v, want %v", got, want)
	}

	// Exactly 04:00 → must advance to NEXT day (strictly after).
	cur = time.Date(2026, 6, 15, 4, 0, 0, 0, loc)
	got = s.Next(cur)
	want = time.Date(2026, 6, 16, 4, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(04:00) = %v, want %v", got, want)
	}

	// After 04:00 → next day 04:00.
	cur = time.Date(2026, 6, 15, 9, 0, 0, 0, loc)
	got = s.Next(cur)
	want = time.Date(2026, 6, 16, 4, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("Next(09:00) = %v, want %v", got, want)
	}
}

func TestPeriodicWatchRenew_ConstructsWorkspaceJob(t *testing.T) {
	ws := uuid.New()
	pj := PeriodicWatchRenew(ws)
	if pj == nil {
		t.Fatal("PeriodicWatchRenew returned nil")
	}
	// Proves it is a usable river.PeriodicJob value of the right type.
	var _ *river.PeriodicJob = pj
}
