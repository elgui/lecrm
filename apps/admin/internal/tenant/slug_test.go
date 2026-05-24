package tenant

import "testing"

// TestValidateSlug covers AC-V1 (Story 8.1) — the regex must accept the
// integrator's real-world tenant slugs and reject every common mistake
// before the slug reaches Postgres.
func TestValidateSlug(t *testing.T) {
	cases := []struct {
		slug      string
		wantValid bool
	}{
		// Positive — from AC-V1 examples.
		{"chauvet79", true},
		{"chauvet-79", true},
		{"acme-001", true},
		{"abc", true},                                  // shortest legal length
		{"abcdefghijklmnopqrstuvwxyz012345", true},     // 32 chars, longest legal length

		// Negative — from AC-V1 examples plus boundary cases.
		{"chauvé-79", false},                            // non-ASCII letter
		{"Chauvet79", false},                            // uppercase
		{"79chauvet", false},                            // digit start
		{"ch", false},                                   // too short
		{"abcdefghijklmnopqrstuvwxyz0123456", false},    // 33 chars, too long
		{"-leadinghyphen", false},                       // hyphen start
		{"trailing-underscore_", false},                 // underscore (not in charset)
		{"has spaces", false},                           // whitespace
		{"", false},                                     // empty
	}
	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			err := ValidateSlug(tc.slug)
			if tc.wantValid && err != nil {
				t.Fatalf("ValidateSlug(%q) returned error %v; want nil", tc.slug, err)
			}
			if !tc.wantValid && err == nil {
				t.Fatalf("ValidateSlug(%q) returned nil; want error", tc.slug)
			}
			if !tc.wantValid {
				se, ok := err.(*StructErr)
				if !ok {
					t.Fatalf("ValidateSlug(%q): expected *StructErr, got %T", tc.slug, err)
				}
				if se.Kind != ErrKindSlugInvalid {
					t.Fatalf("ValidateSlug(%q): wrong kind %q", tc.slug, se.Kind)
				}
			}
		})
	}
}
