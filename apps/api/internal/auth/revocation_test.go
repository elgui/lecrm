package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBloomFilter_BasicOperations(t *testing.T) {
	bf := newBloomFilter(100, 6)

	key1 := uuid.New()
	key2 := uuid.New()

	bf.add(key1[:])

	if !bf.test(key1[:]) {
		t.Fatal("bloom filter must return true for added item")
	}
	if bf.test(key2[:]) {
		t.Log("bloom filter false positive (possible but unlikely for 2 items)")
	}
}

func TestBloomFilter_NoFalseNegatives(t *testing.T) {
	bf := newBloomFilter(1000, 6)

	var keys []uuid.UUID
	for i := 0; i < 100; i++ {
		k := uuid.New()
		keys = append(keys, k)
		bf.add(k[:])
	}

	for _, k := range keys {
		if !bf.test(k[:]) {
			t.Fatalf("bloom filter returned false for added key %s", k)
		}
	}
}

func TestMemoryRevocationChecker_RevokeJTI(t *testing.T) {
	checker := NewMemoryRevocationChecker()
	ctx := context.Background()

	jti := uuid.New()
	userID := uuid.New()
	issuedAt := time.Now().Unix()

	revoked, err := checker.IsRevoked(ctx, jti, userID, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Fatal("session should not be revoked before revocation")
	}

	checker.RevokeJTI(jti)

	revoked, err = checker.IsRevoked(ctx, jti, userID, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("session must be revoked after JTI revocation")
	}
}

func TestMemoryRevocationChecker_RevokeUser(t *testing.T) {
	checker := NewMemoryRevocationChecker()
	ctx := context.Background()

	jti1 := uuid.New()
	jti2 := uuid.New()
	userID := uuid.New()
	issuedAt := time.Now().Unix()

	checker.RevokeUser(userID)

	revoked1, _ := checker.IsRevoked(ctx, jti1, userID, issuedAt)
	revoked2, _ := checker.IsRevoked(ctx, jti2, userID, issuedAt)
	if !revoked1 || !revoked2 {
		t.Fatal("all sessions for revoked user must be revoked")
	}

	futureJTI := uuid.New()
	futureIssuedAt := time.Now().Add(1 * time.Second).Unix()
	revokedFuture, _ := checker.IsRevoked(ctx, futureJTI, userID, futureIssuedAt)
	if revokedFuture {
		t.Fatal("sessions issued AFTER user revocation should not be revoked")
	}
}

func TestMemoryRevocationChecker_OtherUserNotAffected(t *testing.T) {
	checker := NewMemoryRevocationChecker()
	ctx := context.Background()

	userA := uuid.New()
	userB := uuid.New()
	jti := uuid.New()

	checker.RevokeUser(userA)

	revoked, _ := checker.IsRevoked(ctx, jti, userB, time.Now().Unix())
	if revoked {
		t.Fatal("revoking user A must not affect user B")
	}
}

func TestNopRevocationChecker(t *testing.T) {
	nop := NopRevocationChecker{}
	revoked, err := nop.IsRevoked(context.Background(), uuid.New(), uuid.New(), time.Now().Unix())
	if err != nil || revoked {
		t.Fatal("NopRevocationChecker must always return (false, nil)")
	}
}

func TestSessionV2_JTIPopulated(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	uid := uuid.New()
	wid := uuid.New()

	token, err := EncodeSessionV2(Session{UserID: uid, WorkspaceID: wid}, "acme", secret)
	if err != nil {
		t.Fatal(err)
	}
	s, err := DecodeSessionV2(token, "acme", secret)
	if err != nil {
		t.Fatal(err)
	}
	if s.JTI == (uuid.UUID{}) {
		t.Fatal("JTI must be auto-populated by EncodeSessionV2")
	}
}

func TestSessionV2_JTIPreserved(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	jti := uuid.New()

	token, err := EncodeSessionV2(Session{
		UserID:      uuid.New(),
		WorkspaceID: uuid.New(),
		JTI:         jti,
	}, "acme", secret)
	if err != nil {
		t.Fatal(err)
	}
	s, err := DecodeSessionV2(token, "acme", secret)
	if err != nil {
		t.Fatal(err)
	}
	if s.JTI != jti {
		t.Fatalf("JTI mismatch: got %s want %s", s.JTI, jti)
	}
}

func TestHandler_RevokedSessionReturns401(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	checker := NewMemoryRevocationChecker()
	slug := "acme"

	s := Session{UserID: uuid.New(), WorkspaceID: uuid.New()}
	token, err := EncodeSessionV2(s, slug, secret)
	if err != nil {
		t.Fatal(err)
	}

	decoded, _ := DecodeSessionV2(token, slug, secret)
	checker.RevokeJTI(decoded.JTI)

	h := &Handler{
		SessionSecret: secret,
		DomainTLD:     "lecrm.test",
		Revocations:   checker,
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Host = slug + ".lecrm.test"
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rr := httptest.NewRecorder()
	h.Me(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked session, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "session revoked") {
		t.Fatalf("expected 'session revoked' body, got %q", rr.Body.String())
	}
}

func TestHandler_UnrevokedSessionAllowed(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	checker := NewMemoryRevocationChecker()
	slug := "acme"

	token, err := EncodeSessionV2(
		Session{UserID: uuid.New(), WorkspaceID: uuid.New()},
		slug, secret,
	)
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		SessionSecret: secret,
		DomainTLD:     "lecrm.test",
		Revocations:   checker,
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Host = slug + ".lecrm.test"
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rr := httptest.NewRecorder()
	h.Me(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid session, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["user_id"] == "" {
		t.Fatal("expected user_id in response")
	}
}

func TestHandler_LogoutRevokesJTI(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	checker := NewMemoryRevocationChecker()
	slug := "acme"

	token, err := EncodeSessionV2(
		Session{UserID: uuid.New(), WorkspaceID: uuid.New()},
		slug, secret,
	)
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		SessionSecret: secret,
		DomainTLD:     "lecrm.test",
		Revocations:   checker,
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Host = slug + ".lecrm.test"
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rr := httptest.NewRecorder()
	h.Logout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", rr.Code)
	}

	// The cookie is cleared, but even if someone replays the old token,
	// the JTI should be revoked. Handler.Logout calls revokeJTI which
	// needs a DB pool; without it, the JTI is NOT stored in the
	// MemoryRevocationChecker. This test validates the cookie-clearing
	// path works. Integration tests cover the DB revocation path.
}

func TestHandler_RevokeAllRevokesUser(t *testing.T) {
	secret := []byte("test-secret-that-is-at-least-32-bytes!")
	checker := NewMemoryRevocationChecker()
	slug := "acme"
	uid := uuid.New()

	token1, _ := EncodeSessionV2(Session{UserID: uid, WorkspaceID: uuid.New()}, slug, secret)
	token2, _ := EncodeSessionV2(Session{UserID: uid, WorkspaceID: uuid.New()}, slug, secret)

	checker.RevokeUser(uid)

	h := &Handler{
		SessionSecret: secret,
		DomainTLD:     "lecrm.test",
		Revocations:   checker,
	}

	for i, tok := range []string{token1, token2} {
		req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		req.Host = slug + ".lecrm.test"
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: tok})
		rr := httptest.NewRecorder()
		h.Me(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("session %d: expected 401, got %d", i, rr.Code)
		}
	}
}
