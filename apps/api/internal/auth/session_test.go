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
