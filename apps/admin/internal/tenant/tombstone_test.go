package tenant

import (
	"testing"
)

// TestReservedSlugsList verifies the known dangerous infrastructure slugs
// that must be seeded in migration 0005. This is a compile-time check that
// the reserved slug concept exists in the codebase — the actual DB seeding
// is validated by the integration test suite.
func TestReservedSlugsErrorKinds(t *testing.T) {
	if ErrKindSlugReserved == "" {
		t.Fatal("ErrKindSlugReserved must be defined")
	}
	if ErrKindSlugTombstoned == "" {
		t.Fatal("ErrKindSlugTombstoned must be defined")
	}

	err := New(ErrKindSlugReserved, "slug %q is reserved", "admin")
	if err.Kind != ErrKindSlugReserved {
		t.Fatalf("expected kind %q, got %q", ErrKindSlugReserved, err.Kind)
	}

	err = New(ErrKindSlugTombstoned, "slug %q is tombstoned", "old-tenant")
	if err.Kind != ErrKindSlugTombstoned {
		t.Fatalf("expected kind %q, got %q", ErrKindSlugTombstoned, err.Kind)
	}
}
