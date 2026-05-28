package email

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/email/brevo"
)

// --- fakes -----------------------------------------------------------

type fakeProvider struct {
	sent []brevo.SendRequest
	id   string
	err  error
}

func (f *fakeProvider) Send(_ context.Context, r brevo.SendRequest) (brevo.SendResponse, error) {
	f.sent = append(f.sent, r)
	if f.err != nil {
		return brevo.SendResponse{}, f.err
	}
	id := f.id
	if id == "" {
		id = "<msg-1@brevo>"
	}
	return brevo.SendResponse{MessageID: id}, nil
}

type fakeSuppression struct {
	suppressed map[string]bool
	upserts    []struct {
		email, reason string
		at            time.Time
	}
}

func (f *fakeSuppression) Suppressed(_ context.Context, _ string, emails []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, e := range emails {
		if f.suppressed[e] {
			out[e] = true
		}
	}
	return out, nil
}

func (f *fakeSuppression) Upsert(_ context.Context, _ string, email, reason string, at time.Time) error {
	if f.suppressed == nil {
		f.suppressed = map[string]bool{}
	}
	f.suppressed[email] = true
	f.upserts = append(f.upserts, struct {
		email, reason string
		at            time.Time
	}{email, reason, at})
	return nil
}

type fakeAudit struct {
	events []AuditEvent
}

func (f *fakeAudit) WriteAudit(_ context.Context, ev AuditEvent) error {
	f.events = append(f.events, ev)
	return nil
}

type fakeBounceRate struct {
	stats BounceStats
}

func (f *fakeBounceRate) Stats(_ context.Context, _ string, _ time.Duration) (BounceStats, error) {
	return f.stats, nil
}

// --- helpers ---------------------------------------------------------

func makeService(t *testing.T) (*Service, *fakeProvider, *fakeSuppression, *fakeAudit, *fakeBounceRate) {
	t.Helper()
	prov := &fakeProvider{}
	supp := &fakeSuppression{}
	aud := &fakeAudit{}
	br := &fakeBounceRate{}
	svc := &Service{
		Provider:    prov,
		Suppression: supp,
		Audit:       aud,
		BounceRate:  br,
	}
	return svc, prov, supp, aud, br
}

func sampleReq() SendRequest {
	return SendRequest{
		WorkspaceID: uuid.New(),
		Schema:      "workspace_abc",
		ActorType:   ActorHumanAPI,
		ActorUserID: uuid.New(),
		From:        brevo.Address{Email: "noreply@lecrm.eu"},
		To:          []brevo.Address{{Email: "a@example.com"}, {Email: "b@example.com"}},
		Subject:     "test",
		TextContent: "body",
	}
}

// --- tests -----------------------------------------------------------

func TestSend_HappyPath(t *testing.T) {
	svc, prov, _, aud, _ := makeService(t)
	out, err := svc.Send(context.Background(), sampleReq())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.Skipped {
		t.Errorf("did not expect skipped")
	}
	if out.MessageID == "" {
		t.Errorf("no message id")
	}
	if len(prov.sent) != 1 || len(prov.sent[0].To) != 2 {
		t.Errorf("provider not invoked with both recipients")
	}
	if len(aud.events) != 1 || aud.events[0].Event != "email.send.requested" {
		t.Errorf("audit events: %+v", aud.events)
	}
	if aud.events[0].ActorType != ActorHumanAPI {
		t.Errorf("actor_type: %q", aud.events[0].ActorType)
	}
}

func TestSend_SuppressionFiltersRecipient(t *testing.T) {
	svc, prov, supp, aud, _ := makeService(t)
	supp.suppressed = map[string]bool{"a@example.com": true}
	out, err := svc.Send(context.Background(), sampleReq())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(prov.sent) != 1 || len(prov.sent[0].To) != 1 || prov.sent[0].To[0].Email != "b@example.com" {
		t.Errorf("recipient not filtered: %+v", prov.sent)
	}
	if len(out.SkippedRecipients) != 1 || out.SkippedRecipients[0] != "a@example.com" {
		t.Errorf("skipped: %+v", out.SkippedRecipients)
	}
	if len(aud.events) != 1 || aud.events[0].Event != "email.send.requested" {
		t.Errorf("audit: %+v", aud.events)
	}
}

func TestSend_AllRecipientsSuppressed(t *testing.T) {
	svc, prov, supp, aud, _ := makeService(t)
	supp.suppressed = map[string]bool{"a@example.com": true, "b@example.com": true}
	out, err := svc.Send(context.Background(), sampleReq())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !out.Skipped {
		t.Errorf("should be skipped")
	}
	if len(prov.sent) != 0 {
		t.Errorf("provider should not be called")
	}
	if len(aud.events) != 1 || aud.events[0].Event != "email.send.skipped" {
		t.Errorf("audit: %+v", aud.events)
	}
}

func TestSend_AlarmPausesWorkspace(t *testing.T) {
	svc, prov, _, aud, br := makeService(t)
	br.stats = BounceStats{PauseInEffect: true, Rate: 0.05, TotalSends: 1000, BounceLikeN: 50}
	_, err := svc.Send(context.Background(), sampleReq())
	if !errors.Is(err, ErrPausedByAlarm) {
		t.Fatalf("want ErrPausedByAlarm, got %v", err)
	}
	if len(prov.sent) != 0 {
		t.Errorf("provider should not be called when paused")
	}
	if len(aud.events) != 1 || aud.events[0].Event != "email.send.skipped" {
		t.Errorf("audit: %+v", aud.events)
	}
}

func TestSend_InvalidActorRejected(t *testing.T) {
	svc, _, _, _, _ := makeService(t)
	req := sampleReq()
	req.ActorType = "mcp_agent"
	_, err := svc.Send(context.Background(), req)
	if !errors.Is(err, ErrInvalidActor) {
		t.Fatalf("want ErrInvalidActor, got %v", err)
	}
}

func TestSend_ProviderErrorAudited(t *testing.T) {
	svc, prov, _, aud, _ := makeService(t)
	prov.err = errors.New("brevo down")
	_, err := svc.Send(context.Background(), sampleReq())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(aud.events) != 1 || aud.events[0].Event != "email.send.failed" {
		t.Errorf("expected failure audit, got %+v", aud.events)
	}
}

func TestIngestEvent_HardBounceSuppresses(t *testing.T) {
	svc, _, supp, aud, _ := makeService(t)
	wsID := uuid.New()
	ev, _ := brevo.ParseEvent([]byte(`{"event":"hardBounce","email":"x@y","message-id":"<m1>"}`))
	if err := svc.IngestEvent(context.Background(), wsID, "workspace_x", ev); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if !supp.suppressed["x@y"] {
		t.Errorf("hardBounce should have suppressed")
	}
	if len(aud.events) == 0 || aud.events[0].Event != "email.event.received" {
		t.Errorf("audit: %+v", aud.events)
	}
}

func TestIngestEvent_SoftBounceDoesNotSuppress(t *testing.T) {
	svc, _, supp, _, _ := makeService(t)
	ev, _ := brevo.ParseEvent([]byte(`{"event":"softBounce","email":"x@y"}`))
	if err := svc.IngestEvent(context.Background(), uuid.New(), "w", ev); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if supp.suppressed["x@y"] {
		t.Errorf("softBounce should NOT suppress")
	}
}

func TestIngestEvent_DeliveredAuditOnly(t *testing.T) {
	svc, _, supp, aud, _ := makeService(t)
	ev, _ := brevo.ParseEvent([]byte(`{"event":"delivered","email":"x@y","message-id":"<m1>"}`))
	if err := svc.IngestEvent(context.Background(), uuid.New(), "w", ev); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if supp.suppressed["x@y"] {
		t.Errorf("delivered should not suppress")
	}
	if len(aud.events) != 1 {
		t.Errorf("audit count: %d", len(aud.events))
	}
}

func TestIngestEvent_AlarmFires(t *testing.T) {
	svc, _, _, aud, br := makeService(t)
	br.stats = BounceStats{TotalSends: 1000, BounceLikeN: 5, Rate: 0.005}
	ev, _ := brevo.ParseEvent([]byte(`{"event":"hardBounce","email":"x@y"}`))
	if err := svc.IngestEvent(context.Background(), uuid.New(), "w", ev); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	var alarmFired bool
	for _, e := range aud.events {
		if e.Event == "security.email_bounce_rate_high" {
			alarmFired = true
		}
	}
	if !alarmFired {
		t.Errorf("alarm should have fired: events=%+v", aud.events)
	}
}

func TestIngestEvent_AlarmIgnoresSmallSamples(t *testing.T) {
	svc, _, _, aud, br := makeService(t)
	// 1 bounce of 5 → 20% rate but below MinSamplesForAlarm.
	br.stats = BounceStats{TotalSends: 5, BounceLikeN: 1, Rate: 0.2}
	ev, _ := brevo.ParseEvent([]byte(`{"event":"hardBounce","email":"x@y"}`))
	if err := svc.IngestEvent(context.Background(), uuid.New(), "w", ev); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	for _, e := range aud.events {
		if e.Event == "security.email_bounce_rate_high" {
			t.Fatalf("alarm should NOT have fired with %d samples", 5)
		}
	}
}

func TestIngestEvent_UnknownRejected(t *testing.T) {
	svc, _, _, _, _ := makeService(t)
	ev := brevo.Event{Event: "wat", Email: "x@y"}
	if err := svc.IngestEvent(context.Background(), uuid.New(), "w", ev); err == nil {
		t.Fatal("want error for unknown event")
	}
}

func TestShouldTripAlarm_PureLogic(t *testing.T) {
	tests := []struct {
		name string
		s    BounceStats
		want bool
	}{
		{"too few samples", BounceStats{TotalSends: 50, Rate: 0.99}, false},
		{"under threshold", BounceStats{TotalSends: 1000, Rate: 0.0005}, false},
		{"at threshold", BounceStats{TotalSends: 1000, Rate: 0.001}, false},
		{"over threshold", BounceStats{TotalSends: 1000, Rate: 0.0011}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldTripAlarm(tt.s); got != tt.want {
				t.Errorf("ShouldTripAlarm(%+v) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestIsValidSendActor(t *testing.T) {
	if !IsValidSendActor(ActorHumanAPI) || !IsValidSendActor(ActorInternalService) {
		t.Errorf("standard actors must be valid")
	}
	for _, bad := range []ActorType{"mcp_agent", "system", ""} {
		if IsValidSendActor(bad) {
			t.Errorf("%q must be invalid", bad)
		}
	}
}
