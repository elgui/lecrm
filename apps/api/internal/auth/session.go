package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Session is the v0 server-side session shape carried in the
// HMAC-signed session cookie. The cookie is stateless: revocation at v0
// is done by rotating LECRM_SESSION_SECRET. Workspace-scoped Bearer
// service tokens (ADR-009 §4.1) and database-backed revocation land in
// Sprint 7.
//
// Identity is keyed on UserID (core.users.id), NOT on raw `sub` —
// ADR-009 §7.1 binding: the (issuer, sub) tuple lives in core.users and
// is resolved at callback time, so the session never has to carry the
// IDP-specific identifier.
type Session struct {
	UserID      uuid.UUID `json:"uid"`
	WorkspaceID uuid.UUID `json:"wid"`
	IssuedAt    int64     `json:"iat"`
	ExpiresAt   int64     `json:"exp"`
}

// EncodeSession signs s with secret and returns the cookie value
// "<base64url(payload)>.<base64url(hmac)>".
func EncodeSession(s Session, secret []byte) (string, error) {
	if s.UserID == uuid.Nil || s.WorkspaceID == uuid.Nil {
		return "", errors.New("session requires non-zero UserID and WorkspaceID")
	}
	if s.IssuedAt == 0 {
		s.IssuedAt = time.Now().Unix()
	}
	if s.ExpiresAt == 0 {
		s.ExpiresAt = time.Now().Add(MaxAge).Unix()
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encodedPayload + "." + sig, nil
}

// DecodeSession verifies the HMAC, parses the payload, and rejects
// expired sessions. The returned error is intentionally non-specific —
// callers should treat any failure as "no session".
func DecodeSession(value string, secret []byte) (Session, error) {
	var zero Session
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return zero, errors.New("invalid session token format")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return zero, errors.New("session signature invalid")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return zero, fmt.Errorf("session payload not base64url: %w", err)
	}
	var s Session
	if err := json.Unmarshal(payload, &s); err != nil {
		return zero, fmt.Errorf("session payload not JSON: %w", err)
	}
	if time.Now().Unix() > s.ExpiresAt {
		return zero, errors.New("session expired")
	}
	return s, nil
}
