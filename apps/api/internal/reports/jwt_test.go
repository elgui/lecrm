package reports

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var testSecret = []byte("test-secret-test-secret-test-secret-32b")

func TestSignAndVerifyRoundTrip(t *testing.T) {
	wsID := uuid.New()
	token, exp, err := SignEmbedToken(EmbedClaims{WorkspaceID: wsID}, testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if exp.IsZero() {
		t.Fatal("zero expiry")
	}
	got, err := VerifyEmbedToken(token, testSecret)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.WorkspaceID != wsID {
		t.Errorf("workspace_id: got %s want %s", got.WorkspaceID, wsID)
	}
	if got.Audience != CubeAudience {
		t.Errorf("audience: got %q want %q", got.Audience, CubeAudience)
	}
	if got.ExpiresAt == 0 {
		t.Error("exp not populated")
	}
	if got.IssuedAt == 0 {
		t.Error("iat not populated")
	}
}

func TestSignDefaultTTL(t *testing.T) {
	wsID := uuid.New()
	before := time.Now().Add(DefaultTTL - 5*time.Second)
	_, exp, err := SignEmbedToken(EmbedClaims{WorkspaceID: wsID}, testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	after := time.Now().Add(DefaultTTL + 5*time.Second)
	if exp.Before(before) || exp.After(after) {
		t.Errorf("expiry %s not within [%s, %s]", exp, before, after)
	}
}

func TestSignZeroWorkspaceRejected(t *testing.T) {
	_, _, err := SignEmbedToken(EmbedClaims{}, testSecret)
	if err == nil {
		t.Fatal("expected error for zero workspace_id")
	}
}

func TestSignShortSecretRejected(t *testing.T) {
	_, _, err := SignEmbedToken(EmbedClaims{WorkspaceID: uuid.New()}, []byte("short"))
	if err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestVerifyTamperedSignatureRejected(t *testing.T) {
	wsID := uuid.New()
	token, _, err := SignEmbedToken(EmbedClaims{WorkspaceID: wsID}, testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Flip the last character of the signature.
	idx := strings.LastIndex(token, ".") + 1
	tampered := token[:len(token)-1]
	if token[len(token)-1] == 'A' {
		tampered += "B"
	} else {
		tampered += "A"
	}
	if _, err := VerifyEmbedToken(tampered, testSecret); err == nil {
		t.Fatalf("expected signature mismatch (idx=%d)", idx)
	}
}

func TestVerifyWrongSecretRejected(t *testing.T) {
	wsID := uuid.New()
	token, _, err := SignEmbedToken(EmbedClaims{WorkspaceID: wsID}, testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	otherSecret := []byte("nope-nope-nope-nope-nope-nope-nope-nope!!")
	if _, err := VerifyEmbedToken(token, otherSecret); err == nil {
		t.Fatal("expected signature mismatch with wrong secret")
	}
}

func TestVerifyExpiredTokenRejected(t *testing.T) {
	wsID := uuid.New()
	past := time.Now().Add(-time.Hour).Unix()
	token, _, err := SignEmbedToken(EmbedClaims{
		WorkspaceID: wsID,
		IssuedAt:    past,
		ExpiresAt:   past + 60, // expired 59m ago
	}, testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := VerifyEmbedToken(token, testSecret); err == nil {
		t.Fatal("expected expired-token error")
	}
}

func TestVerifyMalformedTokenRejected(t *testing.T) {
	cases := []string{"", "abc", "abc.def", "a.b.c.d"}
	for _, c := range cases {
		if _, err := VerifyEmbedToken(c, testSecret); err == nil {
			t.Errorf("expected error for malformed token %q", c)
		}
	}
}
