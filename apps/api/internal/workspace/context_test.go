package workspace

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestWorkspaceFromContext_RoundTrip(t *testing.T) {
	id := uuid.New()
	in := &Context{ID: id, Slug: "acme", RoleName: "workspace_" + id.String()}

	ctx := WithWorkspace(context.Background(), in)

	out, err := WorkspaceFromContext(ctx)
	if err != nil {
		t.Fatalf("WorkspaceFromContext returned err: %v", err)
	}
	if out.ID != id || out.Slug != "acme" {
		t.Fatalf("round-trip mismatch: got %+v", out)
	}
}

func TestWorkspaceFromContext_Missing(t *testing.T) {
	_, err := WorkspaceFromContext(context.Background())
	if !errors.Is(err, ErrMissingWorkspace) {
		t.Fatalf("expected ErrMissingWorkspace, got %v", err)
	}
}

// TestContextKeyIsUnexportedType is a structural assertion: code in
// another package cannot synthesise a Context value because the key
// type ctxKey is unexported. We can't *prove* that with a Go test
// (it's a compile-time guarantee), but we can guard against accidental
// regression to a string key by checking that a string key with the
// same surface value does NOT roundtrip.
func TestContextKeyIsUnexportedType(t *testing.T) {
	const sneakyKey = "ctxKey{}"
	ws := &Context{ID: uuid.New(), Slug: "acme"}

	ctx := context.WithValue(context.Background(), sneakyKey, ws) //nolint:staticcheck // intentional: this is what we DO NOT want to work
	if _, err := WorkspaceFromContext(ctx); !errors.Is(err, ErrMissingWorkspace) {
		t.Fatalf("string-key shadowing should NOT roundtrip; got err=%v", err)
	}
}
