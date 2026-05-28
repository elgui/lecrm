// Package reports issues the embed JWT consumed by the Cube.dev
// container (deploy/compose/cube.yml). The token is short-lived
// (DefaultTTL) and carries only the workspace_id claim — Cube enforces
// per-workspace isolation by running `SET LOCAL ROLE workspace_<id>_ro`
// before every query, driven by this claim (see deploy/cube/cube.js).
//
// JWT format: RFC 7519, HS256. We hand-roll the encoding (no external
// JWT dep) because the claim shape is closed and the signing surface is
// tiny — pulling in github.com/golang-jwt/jwt for ~30 lines of code is
// the wrong trade.
package reports

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DefaultTTL is the embed-token lifetime. 5 minutes matches the Cube
// scheduledRefreshTimer in deploy/cube/cube.js and bounds the blast
// radius of a leaked token.
const DefaultTTL = 5 * time.Minute

// CubeAudience is the `aud` claim Cube checks. Must remain "cube" to
// stay compatible with the cubejs JWT verifier defaults.
const CubeAudience = "cube"

// EmbedClaims is the minimum payload Cube needs to scope a session.
// workspace_id drives the queryRewrite hook in deploy/cube/cube.js.
type EmbedClaims struct {
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Audience    string    `json:"aud"`
	IssuedAt    int64     `json:"iat"`
	ExpiresAt   int64     `json:"exp"`
}

// SignEmbedToken returns a HS256 JWT carrying claims, signed with
// secret. iat/exp are populated from now/ttl if zero.
func SignEmbedToken(claims EmbedClaims, secret []byte) (string, time.Time, error) {
	if claims.WorkspaceID == uuid.Nil {
		return "", time.Time{}, errors.New("reports: workspace_id required")
	}
	if len(secret) < 32 {
		return "", time.Time{}, errors.New("reports: secret must be at least 32 bytes")
	}
	if claims.Audience == "" {
		claims.Audience = CubeAudience
	}
	now := time.Now()
	if claims.IssuedAt == 0 {
		claims.IssuedAt = now.Unix()
	}
	var exp time.Time
	if claims.ExpiresAt == 0 {
		exp = now.Add(DefaultTTL)
		claims.ExpiresAt = exp.Unix()
	} else {
		exp = time.Unix(claims.ExpiresAt, 0)
	}

	headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("reports: marshal claims: %w", err)
	}

	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signing := header + "." + payload

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signing + "." + sig, exp, nil
}

// VerifyEmbedToken decodes and verifies a token signed by SignEmbedToken.
// Returned only by tests today — the production verifier is Cube itself.
func VerifyEmbedToken(token string, secret []byte) (EmbedClaims, error) {
	var zero EmbedClaims

	// Split into header.payload.signature.
	var header, payload, sig string
	parts := 0
	last := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			switch parts {
			case 0:
				header = token[last:i]
			case 1:
				payload = token[last:i]
			default:
				return zero, errors.New("reports: token has too many segments")
			}
			parts++
			last = i + 1
		}
	}
	if parts != 2 {
		return zero, errors.New("reports: token must have three segments")
	}
	sig = token[last:]

	signing := header + "." + payload
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return zero, errors.New("reports: signature mismatch")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return zero, fmt.Errorf("reports: payload not base64url: %w", err)
	}
	var c EmbedClaims
	if err := json.Unmarshal(payloadJSON, &c); err != nil {
		return zero, fmt.Errorf("reports: payload not JSON: %w", err)
	}

	if c.ExpiresAt > 0 && time.Now().Unix() > c.ExpiresAt {
		return zero, errors.New("reports: token expired")
	}

	return c, nil
}
