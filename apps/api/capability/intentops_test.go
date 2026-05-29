package capability

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// These are the DB-free unit tests for the intent write tools: the
// scope→role write gate, the destructive classification, and the pure
// helpers. The full DB-backed behaviour (fuzzy match, dedup, cross-tenant,
// idempotent replay, audit attribution) lives in
// intentops_integration_test.go behind the `integration` build tag.

func TestMCPWritePrincipal_ScopeGate(t *testing.T) {
	ws := uuid.New()
	cases := []struct {
		name    string
		scopes  []string
		wantErr error
	}{
		{"read-only scope denied", []string{ScopeCRMRead}, ErrReadOnlyScope},
		{"empty scopes denied", nil, ErrForbidden}, // scope-less ⇒ role gate, RoleMember < RoleAdmin
		{"unrecognised scope denied", []string{"foo:bar"}, ErrReadOnlyScope},
		{"write scope allowed", []string{ScopeCRMWrite}, nil},
		{"wildcard allowed", []string{ScopeWildcard}, nil},
		{"mixed read+write allowed", []string{ScopeCRMRead, ScopeCRMWrite}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := MCPWritePrincipal(ws, tc.scopes)
			if p.ActorType != ActorTypeMCPAgent {
				t.Fatalf("ActorType = %q, want %q", p.ActorType, ActorTypeMCPAgent)
			}
			if p.ReadRole != "" {
				t.Fatalf("write principal must not carry a ReadRole, got %q", p.ReadRole)
			}
			if p.Schema != MCPSchemaName(ws) {
				t.Fatalf("Schema = %q, want %q", p.Schema, MCPSchemaName(ws))
			}
			if got := AuthorizeWrite(p); !errors.Is(got, tc.wantErr) {
				t.Fatalf("AuthorizeWrite = %v, want %v", got, tc.wantErr)
			}
		})
	}
}

func TestAdvanceDealDestructive(t *testing.T) {
	now := "today"
	if advanceDealDestructive(AdvanceDealInput{Deal: "d", ToStage: "won"}) {
		t.Fatal("a plain stage move must NOT be destructive")
	}
	if !advanceDealDestructive(AdvanceDealInput{Deal: "d", ToStage: "won", MarkClosedAt: &now}) {
		t.Fatal("closing a deal (mark_closed_at) must be destructive")
	}
}

// A read-only token is denied at the write gate BEFORE any argument
// validation is surfaced — so an unauthorised caller never learns argument
// shape, and (crucially) no DB transaction is opened. The nil-pool Service
// proves no DB access occurs on this path.
func TestLogInteraction_ReadOnlyDeniedBeforeValidation(t *testing.T) {
	s := &Service{} // nil pool: any DB access would panic
	p := MCPWritePrincipal(uuid.New(), []string{ScopeCRMRead})
	_, err := s.LogInteraction(t.Context(), p, LogInteractionInput{ /* empty summary */ }, WriteOptions{})
	if !errors.Is(err, ErrReadOnlyScope) {
		t.Fatalf("read-only token must be denied with ErrReadOnlyScope, got %v", err)
	}
}

func TestCaptureLead_ReadOnlyDeniedBeforeValidation(t *testing.T) {
	s := &Service{}
	p := MCPWritePrincipal(uuid.New(), []string{ScopeCRMRead})
	_, err := s.CaptureLead(t.Context(), p, CaptureLeadInput{ /* empty name+source */ }, WriteOptions{})
	if !errors.Is(err, ErrReadOnlyScope) {
		t.Fatalf("read-only token must be denied with ErrReadOnlyScope, got %v", err)
	}
}

// A write-scoped token with invalid args gets the validation error (and still
// no DB access on the validation short-circuit path).
func TestLogInteraction_WriteScopeSeesValidation(t *testing.T) {
	s := &Service{}
	p := MCPWritePrincipal(uuid.New(), []string{ScopeCRMWrite})
	_, err := s.LogInteraction(t.Context(), p, LogInteractionInput{Summary: ""}, WriteOptions{})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError for empty summary, got %v", err)
	}
}

func TestCaptureLead_WriteScopeSeesValidation(t *testing.T) {
	s := &Service{}
	p := MCPWritePrincipal(uuid.New(), []string{ScopeCRMWrite})
	if _, err := s.CaptureLead(t.Context(), p, CaptureLeadInput{Name: "", Source: "chat"}, WriteOptions{}); !isValidation(err) {
		t.Fatalf("want ValidationError for empty name, got %v", err)
	}
	if _, err := s.CaptureLead(t.Context(), p, CaptureLeadInput{Name: "Ada", Source: ""}, WriteOptions{}); !isValidation(err) {
		t.Fatalf("want ValidationError for empty source, got %v", err)
	}
}

func isValidation(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

func TestSplitName(t *testing.T) {
	cases := []struct{ in, first, last string }{
		{"Ada Lovelace", "Ada", "Lovelace"},
		{"Cher", "Cher", ""},
		{"  Grace  Brewster Hopper ", "Grace", "Brewster Hopper"},
		{"", "", ""},
	}
	for _, c := range cases {
		f, l := splitName(c.in)
		if f != c.first || l != c.last {
			t.Errorf("splitName(%q) = (%q,%q), want (%q,%q)", c.in, f, l, c.first, c.last)
		}
	}
}

func TestClosedAtTime(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	mk := func(s string) *string { return &s }

	if got := closedAtTime(nil, now); !got.Equal(now) {
		t.Fatalf("nil ⇒ now, got %v", got)
	}
	for _, v := range []string{"", "today", "NOW", "garbage"} {
		if got := closedAtTime(mk(v), now); !got.Equal(now) {
			t.Errorf("%q ⇒ now, got %v", v, got)
		}
	}
	got := closedAtTime(mk("2026-01-15"), now)
	want := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("date ⇒ %v, want %v", got, want)
	}
}

func TestNullUUIDPtr(t *testing.T) {
	if nullUUIDPtr(nil).Valid {
		t.Fatal("nil ⇒ invalid NullUUID")
	}
	id := uuid.New()
	got := nullUUIDPtr(&id)
	if !got.Valid || got.UUID != id {
		t.Fatalf("nullUUIDPtr(&id) = %+v, want valid %s", got, id)
	}
}
