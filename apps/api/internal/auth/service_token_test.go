package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateAndVerifyServiceToken_RoundTrip(t *testing.T) {
	plaintext, hash, err := GenerateServiceToken("acme")
	if err != nil {
		t.Fatalf("GenerateServiceToken err: %v", err)
	}
	if !strings.HasPrefix(plaintext, "lecrm_acme_") {
		t.Errorf("plaintext shape: got %q", plaintext)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash shape: got %q", hash)
	}
	if err := VerifyServiceToken(plaintext, hash); err != nil {
		t.Errorf("VerifyServiceToken correct token: %v", err)
	}
	if err := VerifyServiceToken(plaintext+"x", hash); err == nil {
		t.Errorf("VerifyServiceToken tampered token: expected error, got nil")
	}
}

func TestGenerateServiceToken_RequiresSlug(t *testing.T) {
	if _, _, err := GenerateServiceToken(""); err == nil {
		t.Fatalf("expected error for empty slug")
	}
}

func TestGenerateServiceToken_UniquePerCall(t *testing.T) {
	p1, _, _ := GenerateServiceToken("acme")
	p2, _, _ := GenerateServiceToken("acme")
	if p1 == p2 {
		t.Fatalf("two consecutive tokens collided: %q", p1)
	}
}

func TestWorkspaceSlugFromToken(t *testing.T) {
	cases := []struct {
		name      string
		token     string
		wantSlug  string
		wantError bool
	}{
		{"happy", "lecrm_acme_AAAA", "acme", false},
		{"with-hyphen", "lecrm_acme-corp_AAAA", "acme-corp", false},
		{"missing-prefix", "stripe_acme_AAAA", "", true},
		{"missing-suffix", "lecrm_acme_", "", true},
		{"missing-slug", "lecrm__AAAA", "", true},
		{"uppercase-slug", "lecrm_ACME_AAAA", "", true},
		{"underscore-in-slug", "lecrm_a_b_AAAA", "a", false}, // first `_` ends slug, rest is suffix
		{"empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := WorkspaceSlugFromToken(tc.token)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got slug=%q", got)
				}
				if !errors.Is(err, ErrInvalidTokenFormat) {
					t.Fatalf("expected ErrInvalidTokenFormat, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantSlug {
				t.Errorf("slug: got %q want %q", got, tc.wantSlug)
			}
		})
	}
}

func TestVerifyServiceToken_RejectsMalformedHash(t *testing.T) {
	plaintext, _, _ := GenerateServiceToken("acme")
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=99$m=1,t=1,p=1$AAAA$AAAA", // unsupported version
		"$argon2id$v=19$nope$AAAA$AAAA",        // malformed params
		"$bcrypt$2$10$abc",                     // wrong algorithm
	}
	for _, h := range cases {
		if err := VerifyServiceToken(plaintext, h); err == nil {
			t.Errorf("expected error for malformed hash %q", h)
		}
	}
}

func TestFingerprintToken(t *testing.T) {
	plaintext, _, _ := GenerateServiceToken("acme")
	fp := FingerprintToken(plaintext)
	if len(fp) != 16 { // 8 bytes hex
		t.Errorf("fingerprint length: got %d want 16 (%q)", len(fp), fp)
	}
	if FingerprintToken("no-underscore") != "" {
		t.Errorf("malformed token should yield empty fingerprint")
	}
}
