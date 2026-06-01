package crm

// Unit tests for the pure dedup helpers — name normalization, in-memory
// trigram similarity, canonical pair ordering, no-merge exclusion lookup, and
// the survivor/loser field resolvers. No build tag, so they run in the plain
// `go test ./...` gate (the merge correctness itself is covered by the
// Docker-gated dedup_integration_test.go).

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"  Acme,  Inc. ": "acme  inc",  // punctuation stripped, spaces kept (not collapsed), trimmed, lowercased
		"Café Déco":      "café déco",  // accented letters are kept (unicode.IsLetter)
		"A.B.C":          "abc",
		"":               "",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Errorf("normalizeName(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestTrigramSimilarity(t *testing.T) {
	if s := trigramSimilarity("Acme Corp", "Acme Corp"); s != 1.0 {
		t.Errorf("identical strings should score 1.0, got %v", s)
	}
	if s := trigramSimilarity("", ""); s != 1.0 {
		t.Errorf("two empty strings should score 1.0, got %v", s)
	}
	if s := trigramSimilarity("Acme", ""); s != 0.0 {
		t.Errorf("one empty string should score 0.0, got %v", s)
	}
	// Near-identical names should score high; unrelated names low.
	near := trigramSimilarity("Boulangerie Lefevre", "Boulangerie Lefebvre")
	far := trigramSimilarity("Boulangerie Lefevre", "TransAlpes Logistique")
	if near <= far {
		t.Errorf("expected similar > dissimilar: near=%v far=%v", near, far)
	}
	if near < 0.5 {
		t.Errorf("expected high similarity for one-letter typo, got %v", near)
	}
	// Symmetric.
	if a, b := trigramSimilarity("foo bar", "bar foo"), trigramSimilarity("bar foo", "foo bar"); a != b {
		t.Errorf("trigramSimilarity must be symmetric: %v != %v", a, b)
	}
}

func TestCanonicalPair(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	b := uuid.MustParse("00000000-0000-0000-0000-0000000000bb")
	lo1, hi1 := canonicalPair(a, b)
	lo2, hi2 := canonicalPair(b, a) // reversed input
	if lo1 != lo2 || hi1 != hi2 {
		t.Errorf("canonicalPair must be order-independent: (%v,%v) vs (%v,%v)", lo1, hi1, lo2, hi2)
	}
	if lo1.String() >= hi1.String() {
		t.Errorf("canonicalPair must return (lo,hi): %v >= %v", lo1, hi1)
	}
}

func TestIsExcluded(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	b := uuid.MustParse("00000000-0000-0000-0000-0000000000bb")
	lo, hi := canonicalPair(a, b)
	excluded := map[[2]uuid.UUID]bool{{lo, hi}: true}

	if !isExcluded(excluded, a, b) {
		t.Error("excluded pair not detected (a,b)")
	}
	if !isExcluded(excluded, b, a) {
		t.Error("exclusion must be order-independent (b,a)")
	}
	c := uuid.MustParse("00000000-0000-0000-0000-0000000000cc")
	if isExcluded(excluded, a, c) {
		t.Error("non-excluded pair reported as excluded")
	}
}

func TestPickResolvers(t *testing.T) {
	t.Run("generic pick", func(t *testing.T) {
		fields := map[string]string{"first_name": "loser", "last_name": "survivor"}
		if got := pick(fields, "first_name", "S", "L"); got != "L" {
			t.Errorf(`pick with "loser" should return loser value, got %q`, got)
		}
		if got := pick(fields, "last_name", "S", "L"); got != "S" {
			t.Errorf(`pick with "survivor" should return survivor value, got %q`, got)
		}
		if got := pick(fields, "absent", "S", "L"); got != "S" {
			t.Errorf("pick default (key absent) should be survivor, got %q", got)
		}
	})

	t.Run("pickText", func(t *testing.T) {
		s := pgtype.Text{String: "surv", Valid: true}
		l := pgtype.Text{String: "lose", Valid: true}
		if got := pickText(map[string]string{"email": "loser"}, "email", s, l); got.String != "lose" {
			t.Errorf("pickText loser failed: %v", got)
		}
		if got := pickText(map[string]string{}, "email", s, l); got.String != "surv" {
			t.Errorf("pickText default failed: %v", got)
		}
	})

	t.Run("pickNullUUID", func(t *testing.T) {
		s := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		l := uuid.NullUUID{UUID: uuid.New(), Valid: true}
		if got := pickNullUUID(map[string]string{"company_id": "loser"}, "company_id", s, l); got.UUID != l.UUID {
			t.Error("pickNullUUID loser failed")
		}
		if got := pickNullUUID(map[string]string{"company_id": "survivor"}, "company_id", s, l); got.UUID != s.UUID {
			t.Error("pickNullUUID survivor failed")
		}
	})
}
