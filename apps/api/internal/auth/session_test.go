package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSessionRoundTrip(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	uid := uuid.New()
	wid := uuid.New()
	v, err := EncodeSession(Session{UserID: uid, WorkspaceID: wid}, secret)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := DecodeSession(v, secret)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.UserID != uid || got.WorkspaceID != wid {
		t.Fatalf("ids mismatch: got %+v", got)
	}
	if got.IssuedAt == 0 || got.ExpiresAt <= got.IssuedAt {
		t.Fatalf("times not populated: %+v", got)
	}
}

func TestSessionRejectsTamperedSignature(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	v, _ := EncodeSession(Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, secret)
	parts := strings.SplitN(v, ".", 2)
	tampered := parts[0] + ".AAAA" // garbage signature
	if _, err := DecodeSession(tampered, secret); err == nil {
		t.Fatal("decode must reject tampered signature")
	}
}

func TestSessionRejectsTamperedPayload(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	v, _ := EncodeSession(Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, secret)
	parts := strings.SplitN(v, ".", 2)
	if _, err := DecodeSession("AAAA."+parts[1], secret); err == nil {
		t.Fatal("decode must reject mismatched payload+signature")
	}
}

func TestSessionRejectsWrongSecret(t *testing.T) {
	v, _ := EncodeSession(Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	if _, err := DecodeSession(v, []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")); err == nil {
		t.Fatal("decode must reject signature made under a different secret")
	}
}

func TestSessionRejectsExpired(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	s := Session{
		UserID:      uuid.New(),
		WorkspaceID: uuid.New(),
		IssuedAt:    time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt:   time.Now().Add(-1 * time.Hour).Unix(),
	}
	v, _ := EncodeSession(s, secret)
	if _, err := DecodeSession(v, secret); err == nil {
		t.Fatal("decode must reject expired sessions")
	}
}

func TestEncodeRejectsZeroIDs(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	if _, err := EncodeSession(Session{}, secret); err == nil {
		t.Fatal("encode must reject zero UUIDs")
	}
	if _, err := EncodeSession(Session{UserID: uuid.New()}, secret); err == nil {
		t.Fatal("encode must reject zero WorkspaceID")
	}
}

// --- V2 session tests ---

func TestSessionV2RoundTrip(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	uid := uuid.New()
	wid := uuid.New()
	slug := "acme"

	token, err := EncodeSessionV2(Session{UserID: uid, WorkspaceID: wid}, slug, secret)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.HasPrefix(token, "v2.") {
		t.Fatalf("V2 token must start with 'v2.' prefix, got %q", token[:10])
	}
	got, err := DecodeSessionV2(token, slug, secret)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.UserID != uid || got.WorkspaceID != wid {
		t.Fatalf("ids mismatch: got %+v", got)
	}
	if got.IssuedAt == 0 || got.ExpiresAt <= got.IssuedAt {
		t.Fatalf("times not populated: %+v", got)
	}
}

func TestSessionV2_CrossTenantReplay(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	uid := uuid.New()
	wid := uuid.New()

	token, err := EncodeSessionV2(Session{UserID: uid, WorkspaceID: wid}, "acme", secret)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Token issued for "acme" must NOT decode with "evil" slug.
	if _, err := DecodeSessionV2(token, "evil", secret); err == nil {
		t.Fatal("V2 decode must reject cross-tenant replay (acme token decoded with evil slug)")
	}
}

func TestSessionV2_TamperedOuterHMAC(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	token, _ := EncodeSessionV2(Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, "acme", secret)

	// Replace the signature portion with garbage.
	lastDot := strings.LastIndex(token, ".")
	tampered := token[:lastDot+1] + "AAAAAAAAAAAAAAAA"
	if _, err := DecodeSessionV2(tampered, "acme", secret); err == nil {
		t.Fatal("V2 decode must reject tampered outer HMAC")
	}
}

func TestSessionV2_WrongSecret(t *testing.T) {
	token, _ := EncodeSessionV2(
		Session{UserID: uuid.New(), WorkspaceID: uuid.New()},
		"acme",
		[]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	)
	if _, err := DecodeSessionV2(token, "acme", []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")); err == nil {
		t.Fatal("V2 decode must reject token signed with different secret")
	}
}

func TestSessionV2_Expired(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	s := Session{
		UserID:      uuid.New(),
		WorkspaceID: uuid.New(),
		IssuedAt:    time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt:   time.Now().Add(-1 * time.Hour).Unix(),
	}
	token, _ := EncodeSessionV2(s, "acme", secret)
	if _, err := DecodeSessionV2(token, "acme", secret); err == nil {
		t.Fatal("V2 decode must reject expired sessions")
	}
}

func TestSessionV2_RejectsZeroIDs(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	if _, err := EncodeSessionV2(Session{}, "acme", secret); err == nil {
		t.Fatal("V2 encode must reject zero UUIDs")
	}
	if _, err := EncodeSessionV2(Session{UserID: uuid.New()}, "acme", secret); err == nil {
		t.Fatal("V2 encode must reject zero WorkspaceID")
	}
}

func TestSessionV2_RejectsEmptySlug(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	_, err := EncodeSessionV2(Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, "", secret)
	if err == nil {
		t.Fatal("V2 encode must reject empty workspace slug")
	}
}

func TestSessionV2_PerWorkspaceKeyIsolation(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	uid := uuid.New()
	wid := uuid.New()

	token1, _ := EncodeSessionV2(Session{UserID: uid, WorkspaceID: wid}, "acme", secret)
	token2, _ := EncodeSessionV2(Session{UserID: uid, WorkspaceID: wid}, "other", secret)

	// Same session data, different slugs → different tokens (different encryption keys).
	if token1 == token2 {
		t.Fatal("tokens for different workspaces must differ (per-workspace key derivation)")
	}

	// Cross-decode must fail even for identical session data.
	if _, err := DecodeSessionV2(token1, "other", secret); err == nil {
		t.Fatal("acme token must not decode under other slug")
	}
	if _, err := DecodeSessionV2(token2, "acme", secret); err == nil {
		t.Fatal("other token must not decode under acme slug")
	}
}

func TestSessionV1toV2TransparentUpgrade(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	uid := uuid.New()
	wid := uuid.New()

	// Encode as V1 (legacy).
	v1Token, err := EncodeSession(Session{UserID: uid, WorkspaceID: wid}, secret)
	if err != nil {
		t.Fatalf("V1 encode: %v", err)
	}

	// V2 decode should fail (it's a V1 token).
	if _, err := DecodeSessionV2(v1Token, "acme", secret); err == nil {
		t.Fatal("V2 decode must reject V1 token")
	}

	// V1 decode should still work.
	s, err := DecodeSession(v1Token, secret)
	if err != nil {
		t.Fatalf("V1 decode: %v", err)
	}
	if s.UserID != uid || s.WorkspaceID != wid {
		t.Fatalf("V1 decode ids mismatch: %+v", s)
	}

	// Simulate transparent upgrade: re-encode as V2.
	v2Token, err := EncodeSessionV2(s, "acme", secret)
	if err != nil {
		t.Fatalf("V2 re-encode: %v", err)
	}
	got, err := DecodeSessionV2(v2Token, "acme", secret)
	if err != nil {
		t.Fatalf("V2 decode upgraded: %v", err)
	}
	if got.UserID != uid || got.WorkspaceID != wid {
		t.Fatalf("upgraded session ids mismatch: %+v", got)
	}
}

func TestSessionV2_NotV1Decodable(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	token, _ := EncodeSessionV2(Session{UserID: uuid.New(), WorkspaceID: uuid.New()}, "acme", secret)

	// A V2 token must NOT be decodable as V1.
	if _, err := DecodeSession(token, secret); err == nil {
		t.Fatal("V1 decode must reject V2 token")
	}
}
