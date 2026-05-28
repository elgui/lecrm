package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type stubLoader struct {
	candidates []TokenCandidate
	touchedID  uuid.UUID
	loadErr    error
}

func (s *stubLoader) LoadCandidates(_ context.Context, _ uuid.UUID) ([]TokenCandidate, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.candidates, nil
}

func (s *stubLoader) TouchLastUsed(_ context.Context, id uuid.UUID) error {
	s.touchedID = id
	return nil
}

func TestExtractBearer(t *testing.T) {
	cases := map[string]string{
		"":                   "",
		"Bearer abc":         "abc",
		"bearer abc":         "abc",
		"Basic abc":          "",
		"Bearer   spaced   ": "spaced",
	}
	for hdr, want := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if hdr != "" {
			r.Header.Set("Authorization", hdr)
		}
		got := ExtractBearer(r)
		if got != want {
			t.Errorf("header %q: got %q want %q", hdr, got, want)
		}
	}
}

func TestVerifyBearer_HappyPath(t *testing.T) {
	plaintext, hash, err := GenerateServiceToken("acme")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	wsID := uuid.New()
	tokenID := uuid.New()
	loader := &stubLoader{
		candidates: []TokenCandidate{
			{ID: tokenID, Hash: hash, ActorType: "connector", Scopes: []byte(`["*"]`)},
		},
	}

	actor, err := VerifyBearer(context.Background(), loader, wsID, "acme", plaintext)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if actor.TokenID != tokenID {
		t.Errorf("token id: got %s want %s", actor.TokenID, tokenID)
	}
	if actor.ActorType != "connector" {
		t.Errorf("actor_type: got %q", actor.ActorType)
	}
	if loader.touchedID != tokenID {
		t.Errorf("expected TouchLastUsed to be called with %s, got %s", tokenID, loader.touchedID)
	}
}

func TestVerifyBearer_WorkspaceMismatch(t *testing.T) {
	plaintext, hash, _ := GenerateServiceToken("acme")
	loader := &stubLoader{
		candidates: []TokenCandidate{{ID: uuid.New(), Hash: hash, ActorType: "human_api"}},
	}
	// Token embeds "acme" but resolved workspace is "evil-corp".
	_, err := VerifyBearer(context.Background(), loader, uuid.New(), "evil-corp", plaintext)
	if err == nil {
		t.Fatalf("expected workspace mismatch error, got nil")
	}
}

func TestVerifyBearer_NoMatch(t *testing.T) {
	_, hash, _ := GenerateServiceToken("acme")
	// generated plaintext belongs to "acme" but candidate hash is for
	// a DIFFERENT plaintext (we hash a new token then present a fresh one)
	other, _, _ := GenerateServiceToken("acme")
	loader := &stubLoader{
		candidates: []TokenCandidate{{ID: uuid.New(), Hash: hash, ActorType: "human_api"}},
	}
	_, err := VerifyBearer(context.Background(), loader, uuid.New(), "acme", other)
	if err == nil {
		t.Fatalf("expected no-match error")
	}
}

func TestVerifyBearer_LoaderError(t *testing.T) {
	plaintext, _, _ := GenerateServiceToken("acme")
	loader := &stubLoader{loadErr: errors.New("db down")}
	_, err := VerifyBearer(context.Background(), loader, uuid.New(), "acme", plaintext)
	if err == nil {
		t.Fatalf("expected loader error to propagate")
	}
}

func TestHTTPBearerAuthenticator_Authenticate(t *testing.T) {
	plaintext, hash, _ := GenerateServiceToken("acme")
	tokenID := uuid.New()
	loader := &stubLoader{
		candidates: []TokenCandidate{{ID: tokenID, Hash: hash, ActorType: "mcp_agent"}},
	}
	auth := &HTTPBearerAuthenticator{Loader: loader}

	r := httptest.NewRequest(http.MethodGet, "/v1/contacts", nil)
	r.Header.Set("Authorization", "Bearer "+plaintext)

	ctx, err := auth.Authenticate(r, uuid.New(), "acme")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	actor, ok := BearerActorFromContext(ctx)
	if !ok {
		t.Fatalf("BearerActor missing from returned context")
	}
	if actor.ActorType != "mcp_agent" {
		t.Errorf("actor type: got %q", actor.ActorType)
	}
}

func TestHTTPBearerAuthenticator_NoHeader_Passthrough(t *testing.T) {
	// Missing Authorization → middleware doesn't call us. But if the
	// header is present and malformed (not "Bearer …"), we return
	// (nil, nil) so the request falls through to cookie auth.
	auth := &HTTPBearerAuthenticator{Loader: &stubLoader{}}
	r := httptest.NewRequest(http.MethodGet, "/v1/contacts", nil)
	r.Header.Set("Authorization", "Basic abc")
	ctx, err := auth.Authenticate(r, uuid.New(), "acme")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if ctx != nil {
		t.Fatalf("expected nil ctx for malformed header passthrough")
	}
}
