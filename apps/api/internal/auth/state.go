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
)

// stateMaxAge bounds how long the state cookie remains valid. Ten
// minutes is the OAuth 2.1 recommendation: long enough for slow IdP
// login (MFA prompts), short enough to invalidate abandoned flows.
const stateMaxAge = 10 * time.Minute

// EncodeAuthRequest signs an AuthRequest with the state secret for
// transport via the state cookie.
func EncodeAuthRequest(req AuthRequest, secret []byte) (string, error) {
	if req.State == "" || req.CodeVerifier == "" || req.WorkspaceSubdomain == "" || req.RedirectURI == "" {
		return "", errors.New("AuthRequest requires state, code_verifier, workspace, and redirect_uri")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(enc))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return enc + "." + sig, nil
}

// DecodeAuthRequest verifies the HMAC and parses the AuthRequest. It
// rejects requests older than stateMaxAge.
func DecodeAuthRequest(value string, secret []byte) (AuthRequest, error) {
	var zero AuthRequest
	parts := strings.SplitN(value, ".", 2)
	if len(parts) != 2 {
		return zero, errors.New("invalid state token format")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return zero, errors.New("state signature invalid")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return zero, fmt.Errorf("state payload not base64url: %w", err)
	}
	var req AuthRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return zero, fmt.Errorf("state payload not JSON: %w", err)
	}
	if time.Since(req.IssuedAt) > stateMaxAge {
		return zero, errors.New("state expired")
	}
	return req, nil
}
