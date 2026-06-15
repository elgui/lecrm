package gmailreply

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// fakeValidator returns a fixed email/err regardless of input, recording the
// audience it was asked to check.
type fakeValidator struct {
	email    string
	err      error
	gotAud   string
	gotToken string
}

func (v *fakeValidator) Validate(_ context.Context, token, aud string) (string, error) {
	v.gotToken = token
	v.gotAud = aud
	return v.email, v.err
}

type fakeResolver struct {
	target Target
	err    error
	gotEml string
}

func (r *fakeResolver) ResolveByEmail(_ context.Context, email string) (Target, error) {
	r.gotEml = email
	return r.target, r.err
}

type fakeEnqueuer struct {
	calls []PollMailboxArgs
	err   error
}

func (e *fakeEnqueuer) EnqueuePollMailbox(_ context.Context, args PollMailboxArgs) error {
	e.calls = append(e.calls, args)
	return e.err
}

func newPushRequest(t *testing.T, bearer string, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, PushRoute, bytesReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

func bytesReader(b []byte) io.Reader { return &sliceReader{b: b} }

type sliceReader struct {
	b []byte
	i int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func baseHandler(v TokenValidator, r ConnectionResolver, e MailboxPollEnqueuer) *PushHandler {
	return &PushHandler{
		Validator:      v,
		Audience:       "https://api.lecrm.fr/v1/webhooks/gmail/push",
		ServiceAccount: "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com",
		Resolver:       r,
		Enqueuer:       e,
		Logger:         quietLogger(),
	}
}

func TestPush_MissingToken_401(t *testing.T) {
	h := baseHandler(&fakeValidator{}, &fakeResolver{}, &fakeEnqueuer{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "", encodePush(t, `{"emailAddress":"a@b","historyId":1}`, true)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPush_InvalidToken_401(t *testing.T) {
	enq := &fakeEnqueuer{}
	h := baseHandler(&fakeValidator{err: ErrInvalidToken}, &fakeResolver{}, enq)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "bad.jwt", encodePush(t, `{"emailAddress":"a@b","historyId":1}`, true)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if len(enq.calls) != 0 {
		t.Fatalf("invalid token must not enqueue, got %d", len(enq.calls))
	}
}

func TestPush_WrongServiceAccount_401(t *testing.T) {
	// Token is valid but minted for an unexpected SA → reject.
	h := baseHandler(&fakeValidator{email: "someone-else@evil.iam.gserviceaccount.com"}, &fakeResolver{}, &fakeEnqueuer{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "good.jwt", encodePush(t, `{"emailAddress":"a@b","historyId":1}`, true)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPush_MalformedBody_400(t *testing.T) {
	h := baseHandler(&fakeValidator{email: "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com"}, &fakeResolver{}, &fakeEnqueuer{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "good.jwt", []byte("not-json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPush_UnknownMailbox_204_NoEnqueue(t *testing.T) {
	enq := &fakeEnqueuer{}
	h := baseHandler(
		&fakeValidator{email: "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com"},
		&fakeResolver{err: ErrNoConnection},
		enq,
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "good.jwt", encodePush(t, `{"emailAddress":"stranger@x","historyId":1}`, true)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (ack unknown mailbox)", rec.Code)
	}
	if len(enq.calls) != 0 {
		t.Fatalf("unknown mailbox must not enqueue, got %d", len(enq.calls))
	}
}

func TestPush_Success_EnqueuesMailboxScan(t *testing.T) {
	ws, user := uuid.New(), uuid.New()
	enq := &fakeEnqueuer{}
	v := &fakeValidator{email: "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com"}
	res := &fakeResolver{target: Target{WorkspaceID: ws, UserID: user, EmailAddress: "rep@example.com"}}
	h := baseHandler(v, res, enq)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "good.jwt", encodePush(t, `{"emailAddress":"rep@example.com","historyId":555}`, true)))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if v.gotAud != "https://api.lecrm.fr/v1/webhooks/gmail/push" {
		t.Errorf("validator audience = %q", v.gotAud)
	}
	if res.gotEml != "rep@example.com" {
		t.Errorf("resolver email = %q", res.gotEml)
	}
	if len(enq.calls) != 1 {
		t.Fatalf("enqueue calls = %d, want 1", len(enq.calls))
	}
	got := enq.calls[0]
	if got.WorkspaceID != ws || got.UserID != user {
		t.Errorf("enqueued target = %s/%s, want %s/%s", got.WorkspaceID, got.UserID, ws, user)
	}
	if got.HistoryID != 555 {
		t.Errorf("enqueued historyId = %d, want 555", got.HistoryID)
	}
}

func TestPush_EnqueueFailure_500(t *testing.T) {
	enq := &fakeEnqueuer{err: context.DeadlineExceeded}
	h := baseHandler(
		&fakeValidator{email: "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com"},
		&fakeResolver{target: Target{WorkspaceID: uuid.New(), UserID: uuid.New()}},
		enq,
	)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newPushRequest(t, "good.jwt", encodePush(t, `{"emailAddress":"rep@example.com","historyId":1}`, true)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (Pub/Sub retries transient enqueue failure)", rec.Code)
	}
}
