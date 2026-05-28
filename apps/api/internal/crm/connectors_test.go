package crm

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
)

func TestValidateConnectorEvent(t *testing.T) {
	const slug = "gbconsult"
	cases := []struct {
		name     string
		ev       connectorEvent
		urlSrc   string
		wantCode int
	}{
		{
			name:     "valid",
			ev:       connectorEvent{Event: evtCandidateEnriched, IdempotencyKey: "k1", Source: "chatboting", Workspace: slug},
			urlSrc:   "chatboting",
			wantCode: http.StatusOK,
		},
		{
			name:     "unknown event",
			ev:       connectorEvent{Event: "candidate.exploded", IdempotencyKey: "k1"},
			urlSrc:   "chatboting",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing idempotency key",
			ev:       connectorEvent{Event: evtInvitationSent},
			urlSrc:   "chatboting",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "source mismatch",
			ev:       connectorEvent{Event: evtInvitationSent, IdempotencyKey: "k1", Source: "gmail"},
			urlSrc:   "chatboting",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "cross-tenant workspace mismatch",
			ev:       connectorEvent{Event: evtInvitationSent, IdempotencyKey: "k1", Workspace: "other-tenant"},
			urlSrc:   "chatboting",
			wantCode: http.StatusForbidden,
		},
		{
			name:     "empty workspace allowed (defers to token scope)",
			ev:       connectorEvent{Event: evtInvitationSent, IdempotencyKey: "k1"},
			urlSrc:   "chatboting",
			wantCode: http.StatusOK,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := validateConnectorEvent(&c.ev, c.urlSrc, slug)
			if got != c.wantCode {
				t.Fatalf("validateConnectorEvent = %d, want %d", got, c.wantCode)
			}
		})
	}
}

func TestAuthorizeConnector(t *testing.T) {
	cases := []struct {
		name    string
		actor   *auth.BearerActor
		present bool
		want    int
	}{
		{"no actor", nil, false, http.StatusUnauthorized},
		{"nil actor present flag", nil, true, http.StatusUnauthorized},
		{"wildcard scope", &auth.BearerActor{Scopes: []string{"*"}}, true, http.StatusOK},
		{"exact scope", &auth.BearerActor{Scopes: []string{"connector.push_events"}}, true, http.StatusOK},
		{"extra scopes incl connector", &auth.BearerActor{Scopes: []string{"crm.read", "connector.push_events"}}, true, http.StatusOK},
		{"wrong scope", &auth.BearerActor{Scopes: []string{"crm.read"}}, true, http.StatusForbidden},
		{"empty scopes", &auth.BearerActor{Scopes: []string{}}, true, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := authorizeConnector(c.actor, c.present); got != c.want {
				t.Fatalf("authorizeConnector = %d, want %d", got, c.want)
			}
		})
	}
}

func TestKnownConnectorEventsCount(t *testing.T) {
	// ADR-011 §4 defines 7 event types; guard against accidental drift.
	if len(knownConnectorEvents) != 7 {
		t.Fatalf("expected 7 connector event types, got %d", len(knownConnectorEvents))
	}
}

func TestRequireConnectorScope_NoActor401(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/chatboting/events", nil)
	called := false
	h := RequireConnectorScope(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(rec, req)
	if called {
		t.Fatal("handler should not run without a bearer actor")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireConnectorScope_WrongScope403(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/chatboting/events", nil)
	req = req.WithContext(auth.WithBearerActor(req.Context(), &auth.BearerActor{Scopes: []string{"crm.read"}}))
	called := false
	h := RequireConnectorScope(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(rec, req)
	if called {
		t.Fatal("handler should not run with insufficient scope")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireConnectorScope_ValidPassesThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/chatboting/events", nil)
	req = req.WithContext(auth.WithBearerActor(req.Context(), &auth.BearerActor{Scopes: []string{"connector.push_events"}}))
	called := false
	h := RequireConnectorScope(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler should run for a connector.push_events token")
	}
}
