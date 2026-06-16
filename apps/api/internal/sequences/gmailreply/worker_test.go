package gmailreply

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

// depsWith builds a resolved Deps wired to the given fakes plus a recording
// transitioner, for white-box orchestration tests.
func depsWith(tx *fakeTx, cli HistoryClient, cls Classifier, tr *recordingTransitioner) Deps {
	d := Deps{
		Acquirer:   &stubAcquirer{tx: tx},
		Clients:    &fakeClientFactory{cli: cli},
		Classifier: cls,
		Logger:     quietLogger(),
		transition: tr.fn,
		now:        func() time.Time { return time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC) },
	}
	return d.resolved()
}

func TestPollMailbox_CorrelatesReply_TransitionsReplyReceived(t *testing.T) {
	enr := uuid.New()
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 100}

	tx := &fakeTx{
		cursorRaw: []byte(`{"history_id":100}`),
		matched: []MatchedStep{
			{RFCMessageID: "sent@lecrm", EnrollmentID: enr, StepIndex: 1, State: sequences.StateWaitingReply},
		},
	}
	cli := &fakeClient{
		msgs:   []InboundMessage{{RFC822MessageID: "reply@gmail", InReplyTo: "<sent@lecrm>"}},
		newHID: 150,
	}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if len(tr.calls) != 1 {
		t.Fatalf("transition calls = %d, want 1", len(tr.calls))
	}
	got := tr.calls[0]
	if got.enrollmentID != enr {
		t.Errorf("transition enrollment = %s, want %s", got.enrollmentID, enr)
	}
	if got.to != sequences.StateReplyReceived {
		t.Errorf("transition to = %s, want reply_received", got.to)
	}
	// cursor advanced to newHID.
	e, ok := tx.execMatching("UPDATE sync_connections")
	if !ok {
		t.Fatal("cursor not saved")
	}
	if s, _ := e.args[0].(string); !strings.Contains(s, `"history_id":150`) {
		t.Errorf("cursor saved = %v, want history_id 150", e.args[0])
	}
}

func TestPollMailbox_OOOClassification_TransitionsOOODetected(t *testing.T) {
	enr := uuid.New()
	returns := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 10}

	tx := &fakeTx{
		cursorRaw: []byte(`{"history_id":10}`),
		matched:   []MatchedStep{{RFCMessageID: "sent@lecrm", EnrollmentID: enr, State: sequences.StateWaitingReply}},
	}
	cli := &fakeClient{msgs: []InboundMessage{{RFC822MessageID: "ooo@gmail", InReplyTo: "<sent@lecrm>"}}, newHID: 11}
	cls := &stubClassifier{cls: Classification{Category: "ooo", Confidence: 0.9, IsOOO: true, OOOReturnsAt: &returns}}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, cls, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if cls.calls != 1 {
		t.Errorf("classifier calls = %d, want 1", cls.calls)
	}
	if len(tr.calls) != 1 || tr.calls[0].to != sequences.StateOOODetected {
		t.Fatalf("expected one ooo_detected transition, got %+v", tr.calls)
	}
}

// TestPollMailbox_IllegalTransition_SkipsReplyAndAdvancesCursor: a benign race
// (the enrollment already left waiting_reply, so the reply-transition is illegal)
// must NOT roll back the batch — the reply is skipped and the cursor still
// advances, so the scanned window is not endlessly reprocessed. (PR #17 review #2)
func TestPollMailbox_IllegalTransition_SkipsReplyAndAdvancesCursor(t *testing.T) {
	enr := uuid.New()
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 100}

	tx := &fakeTx{
		cursorRaw: []byte(`{"history_id":100}`),
		matched: []MatchedStep{
			{RFCMessageID: "sent@lecrm", EnrollmentID: enr, StepIndex: 1, State: sequences.StateWaitingReply},
		},
	}
	cli := &fakeClient{
		msgs:   []InboundMessage{{RFC822MessageID: "reply@gmail", InReplyTo: "<sent@lecrm>"}},
		newHID: 150,
	}
	// The transitioner reports an illegal transition (the loser of the race).
	tr := &recordingTransitioner{err: sequences.ErrInvalidTransition}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox should swallow a benign illegal transition, got: %v", err)
	}
	// The cursor still advanced despite the skipped reply — the window is not
	// reprocessed on the next push.
	e, ok := tx.execMatching("UPDATE sync_connections")
	if !ok {
		t.Fatal("cursor not saved after a skipped reply")
	}
	if s, _ := e.args[0].(string); !strings.Contains(s, `"history_id":150`) {
		t.Errorf("cursor saved = %v, want history_id 150", e.args[0])
	}
}

func TestPollMailbox_NoMessages_AdvancesCursorOnly(t *testing.T) {
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 100}
	tx := &fakeTx{cursorRaw: []byte(`{"history_id":100}`)}
	cli := &fakeClient{msgs: nil, newHID: 120}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if len(tr.calls) != 0 {
		t.Fatalf("no replies → no transitions, got %d", len(tr.calls))
	}
	if _, ok := tx.execMatching("UPDATE sync_connections"); !ok {
		t.Fatal("cursor should still advance to newHID")
	}
}

func TestPollMailbox_NoCursorAdvance_DoesNotWriteCursor(t *testing.T) {
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 100}
	tx := &fakeTx{cursorRaw: []byte(`{"history_id":100}`)}
	cli := &fakeClient{msgs: nil, newHID: 100} // no advance
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if _, ok := tx.execMatching("UPDATE sync_connections"); ok {
		t.Fatal("cursor must not be rewritten when newHID does not advance")
	}
}

func TestPollMailbox_HistoryGap_Rebaselines(t *testing.T) {
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 5}
	tx := &fakeTx{cursorRaw: []byte(`{"history_id":5}`)}
	cli := &fakeClient{sinceErr: ErrHistoryGap, watchHID: 9999}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if cli.watchCalls != 1 {
		t.Errorf("re-baseline should call Watch once, got %d", cli.watchCalls)
	}
	if len(tr.calls) != 0 {
		t.Errorf("gap re-baseline must not transition, got %d", len(tr.calls))
	}
	e, ok := tx.execMatching("UPDATE sync_connections")
	if !ok {
		t.Fatal("re-baseline should persist the fresh cursor")
	}
	if s, _ := e.args[0].(string); !strings.Contains(s, `"history_id":9999`) {
		t.Errorf("re-baseline cursor = %v, want 9999", e.args[0])
	}
}

func TestPollMailbox_NoCursor_BaselinesFromNotification(t *testing.T) {
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 700}
	tx := &fakeTx{cursorNoRow: true} // no persisted cursor
	cli := &fakeClient{msgs: nil, newHID: 700}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if cli.sinceStart != 700 {
		t.Errorf("scan start = %d, want notification baseline 700", cli.sinceStart)
	}
	if cli.watchCalls != 0 {
		t.Errorf("baseline from notification should not call Watch, got %d", cli.watchCalls)
	}
}

func TestPollMailbox_NoCursorNoHistoryID_BaselinesViaWatch(t *testing.T) {
	args := PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 0}
	tx := &fakeTx{cursorNoRow: true}
	cli := &fakeClient{watchHID: 4242}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, args); err != nil {
		t.Fatalf("pollMailbox: %v", err)
	}
	if cli.watchCalls != 1 {
		t.Errorf("no baseline at all should establish one via Watch, got %d calls", cli.watchCalls)
	}
	if cli.sinceCalls != 0 {
		t.Errorf("should not scan history without a baseline, got %d", cli.sinceCalls)
	}
	e, ok := tx.execMatching("UPDATE sync_connections")
	if !ok {
		t.Fatalf("watch baseline cursor not persisted: %+v", tx.execs)
	}
	cursor, isStr := e.args[0].(string)
	if !isStr || !strings.Contains(cursor, `"history_id":4242`) {
		t.Fatalf("watch baseline cursor not persisted: %+v", tx.execs)
	}
}

func TestWatchRenew_RenewsActiveConnections(t *testing.T) {
	ws := uuid.New()
	user := uuid.New()
	tx := &fakeTx{conns: []connRow{{
		id:       uuid.New(),
		settings: []byte(`{"user_id":"` + user.String() + `","email_address":"rep@example.com"}`),
		cursor:   []byte(`{"history_id":50}`), // existing cursor → must NOT be overwritten
	}}}
	cli := &fakeClient{watchHID: 99999, watchExp: time.Now().Add(7 * 24 * time.Hour)}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.watchRenew(context.Background(), tx, WatchRenewArgs{WorkspaceID: ws}); err != nil {
		t.Fatalf("watchRenew: %v", err)
	}
	if cli.watchCalls != 1 {
		t.Errorf("watch calls = %d, want 1", cli.watchCalls)
	}
	// Existing cursor (50) must be preserved — re-arming the watch must not skip
	// unscanned history.
	if _, ok := tx.execMatching("UPDATE sync_connections"); ok {
		t.Error("watch_renew must not overwrite an existing cursor")
	}
}

func TestWatchRenew_AdoptsBaselineWhenNoCursor(t *testing.T) {
	ws := uuid.New()
	user := uuid.New()
	tx := &fakeTx{conns: []connRow{{
		id:       uuid.New(),
		settings: []byte(`{"user_id":"` + user.String() + `"}`),
		cursor:   nil, // no cursor yet
	}}}
	cli := &fakeClient{watchHID: 321, watchExp: time.Now()}
	tr := &recordingTransitioner{}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.watchRenew(context.Background(), tx, WatchRenewArgs{WorkspaceID: ws}); err != nil {
		t.Fatalf("watchRenew: %v", err)
	}
	e, ok := tx.execMatching("UPDATE sync_connections")
	if !ok {
		t.Fatalf("first-time baseline should be persisted, execs=%+v", tx.execs)
	}
	cursor, isStr := e.args[0].(string)
	if !isStr || !strings.Contains(cursor, `"history_id":321`) {
		t.Fatalf("first-time baseline should be persisted, execs=%+v", tx.execs)
	}
}

func TestWatchRenew_NoConnections_NoError(t *testing.T) {
	tx := &fakeTx{conns: nil}
	d := depsWith(tx, &fakeClient{}, DefaultClassifier{}, &recordingTransitioner{})
	if err := d.watchRenew(context.Background(), tx, WatchRenewArgs{WorkspaceID: uuid.New()}); err != nil {
		t.Fatalf("watchRenew with no connections should be a no-op, got %v", err)
	}
}

// TestWatchRenew_WatchError_ReturnsErrorForRetry: a failed users.watch() must
// surface as an error so river retries the renewal — otherwise a transient
// Google blip could silently let a watch lapse and drop detection.
func TestWatchRenew_WatchError_ReturnsErrorForRetry(t *testing.T) {
	user := uuid.New()
	tx := &fakeTx{conns: []connRow{{
		id:       uuid.New(),
		settings: []byte(`{"user_id":"` + user.String() + `"}`),
		cursor:   []byte(`{"history_id":7}`),
	}}}
	cli := &fakeClient{watchErr: errors.New("google 503")}
	d := depsWith(tx, cli, DefaultClassifier{}, &recordingTransitioner{})

	if err := d.watchRenew(context.Background(), tx, WatchRenewArgs{WorkspaceID: uuid.New()}); err == nil {
		t.Fatal("watch failure should be returned so river retries the renewal")
	}
	if cli.watchCalls != 1 {
		t.Errorf("watch calls = %d, want 1", cli.watchCalls)
	}
}

// TestWatchRenewWorker_Work_CommitsViaAcquirer proves the river Work wrapper
// acquires the workspace tx, renews, and commits on success.
func TestWatchRenewWorker_Work_CommitsViaAcquirer(t *testing.T) {
	ws := uuid.New()
	tx := &fakeTx{conns: nil} // no connections → no-op success
	acq := &stubAcquirer{tx: tx}
	d := Deps{
		Acquirer: acq,
		Clients:  &fakeClientFactory{cli: &fakeClient{}},
		Logger:   quietLogger(),
	}.resolved()
	w := &watchRenewWorker{deps: d}

	job := &river.Job[WatchRenewArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   WatchRenewArgs{WorkspaceID: ws},
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(acq.gotWS) != 1 || acq.gotWS[0] != ws {
		t.Errorf("AcquireTx workspace = %v, want %s", acq.gotWS, ws)
	}
	if !tx.committed {
		t.Error("successful Work should commit the tx")
	}
}

// TestPollMailboxWorker_Work_CommitsViaAcquirer proves the river Work wrapper
// acquires the workspace tx, runs the scan, and commits on success.
func TestPollMailboxWorker_Work_CommitsViaAcquirer(t *testing.T) {
	ws := uuid.New()
	tx := &fakeTx{cursorRaw: []byte(`{"history_id":1}`)}
	acq := &stubAcquirer{tx: tx}
	d := Deps{
		Acquirer:   acq,
		Clients:    &fakeClientFactory{cli: &fakeClient{newHID: 1}},
		Classifier: DefaultClassifier{},
		Logger:     quietLogger(),
		transition: (&recordingTransitioner{}).fn,
	}.resolved()
	w := &pollMailboxWorker{deps: d}

	job := &river.Job[PollMailboxArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 3},
		Args:   PollMailboxArgs{WorkspaceID: ws, UserID: uuid.New(), HistoryID: 1},
	}
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(acq.gotWS) != 1 || acq.gotWS[0] != ws {
		t.Errorf("AcquireTx workspace = %v, want %s", acq.gotWS, ws)
	}
	if !tx.committed {
		t.Error("successful Work should commit the tx")
	}
}

// TestRegisterWorkers_Registers proves both Gmail workers register on a bundle
// without panicking (distinct kinds).
func TestRegisterWorkers_Registers(t *testing.T) {
	workers := river.NewWorkers()
	RegisterWorkers(workers, Deps{
		Acquirer: &stubAcquirer{},
		Clients:  &fakeClientFactory{cli: &fakeClient{}},
	})
	// AddWorker panics on duplicate/invalid; reaching here means both registered.
}

func TestPollMailbox_TransitionError_Propagates(t *testing.T) {
	enr := uuid.New()
	tx := &fakeTx{
		cursorRaw: []byte(`{"history_id":1}`),
		matched:   []MatchedStep{{RFCMessageID: "s@x", EnrollmentID: enr, State: sequences.StateWaitingReply}},
	}
	cli := &fakeClient{msgs: []InboundMessage{{RFC822MessageID: "r@x", InReplyTo: "<s@x>"}}, newHID: 2}
	tr := &recordingTransitioner{err: errors.New("boom")}
	d := depsWith(tx, cli, DefaultClassifier{}, tr)

	if err := d.pollMailbox(context.Background(), tx, PollMailboxArgs{WorkspaceID: uuid.New(), UserID: uuid.New(), HistoryID: 1}); err == nil {
		t.Fatal("transition error should propagate (fail-closed, river retries)")
	}
	// Cursor must NOT advance when a transition failed (whole tx rolls back).
	if _, ok := tx.execMatching("UPDATE sync_connections"); ok {
		t.Error("cursor must not be saved after a failed transition")
	}
}
