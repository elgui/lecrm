package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/hkdf"
)

const (
	v2Prefix    = "v2."
	hkdfInfoKey = "lecrm-session-v2"
)

type innerPayload struct {
	UserID      uuid.UUID `json:"uid"`
	WorkspaceID uuid.UUID `json:"wid"`
	JTI         uuid.UUID `json:"jti"`
	IssuedAt    int64     `json:"iat"`
	ExpiresAt   int64     `json:"exp"`
}

// EncodeSessionV2 produces a two-layer token:
//
//	v2.<base64url(outer)>.<base64url(hmac)>
//
// Outer layer (JSON, signed by HMAC-SHA256):
//
//	{slug, exp, inner_hash} — verifiable without decryption.
//
// Inner payload (AES-256-GCM, encrypted with per-workspace derived key):
//
//	{uid, wid, iat, exp}
//
// Note: s is passed by value. Auto-populated fields (IssuedAt, ExpiresAt,
// JTI) are set on the copy, not the caller's original. To retrieve the
// final values, decode the returned token.
func EncodeSessionV2(s Session, workspaceSlug string, secret []byte) (string, error) {
	if s.UserID == uuid.Nil || s.WorkspaceID == uuid.Nil {
		return "", errors.New("session requires non-zero UserID and WorkspaceID")
	}
	if workspaceSlug == "" {
		return "", errors.New("workspace slug required for V2 encoding")
	}
	if s.IssuedAt == 0 {
		s.IssuedAt = time.Now().Unix()
	}
	if s.ExpiresAt == 0 {
		s.ExpiresAt = time.Now().Add(MaxAge).Unix()
	}
	if s.JTI == uuid.Nil {
		s.JTI = uuid.New()
	}

	inner := innerPayload(s)
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return "", fmt.Errorf("marshal inner: %w", err)
	}

	encKey, err := deriveKey(secret, workspaceSlug)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}

	ciphertext, err := encryptAESGCM(encKey, innerJSON)
	if err != nil {
		return "", fmt.Errorf("encrypt inner: %w", err)
	}

	innerHash := sha256.Sum256(ciphertext)

	type outerClaims struct {
		Slug      string `json:"slug"`
		ExpiresAt int64  `json:"exp"`
		InnerHash string `json:"ih"`
	}
	outer := outerClaims{
		Slug:      workspaceSlug,
		ExpiresAt: s.ExpiresAt,
		InnerHash: base64.RawURLEncoding.EncodeToString(innerHash[:]),
	}
	outerJSON, err := json.Marshal(outer)
	if err != nil {
		return "", fmt.Errorf("marshal outer: %w", err)
	}

	encodedOuter := base64.RawURLEncoding.EncodeToString(outerJSON)
	encodedInner := base64.RawURLEncoding.EncodeToString(ciphertext)
	payload := encodedOuter + "|" + encodedInner

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return v2Prefix + payload + "." + sig, nil
}

// DecodeSessionV2 verifies the outer HMAC, checks workspace binding,
// decrypts the inner payload, and rejects expired sessions.
func DecodeSessionV2(token string, workspaceSlug string, secret []byte) (Session, error) {
	var zero Session

	if !strings.HasPrefix(token, v2Prefix) {
		return zero, errors.New("not a V2 token")
	}
	token = strings.TrimPrefix(token, v2Prefix)

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return zero, errors.New("invalid V2 token format")
	}
	payload, sig := parts[0], parts[1]

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return zero, errors.New("V2 signature invalid")
	}

	outerInner := strings.SplitN(payload, "|", 2)
	if len(outerInner) != 2 {
		return zero, errors.New("invalid V2 payload structure")
	}
	encodedOuter, encodedInner := outerInner[0], outerInner[1]

	outerJSON, err := base64.RawURLEncoding.DecodeString(encodedOuter)
	if err != nil {
		return zero, fmt.Errorf("outer not base64url: %w", err)
	}
	var outer struct {
		Slug      string `json:"slug"`
		ExpiresAt int64  `json:"exp"`
		InnerHash string `json:"ih"`
	}
	if err := json.Unmarshal(outerJSON, &outer); err != nil {
		return zero, fmt.Errorf("outer not JSON: %w", err)
	}

	if outer.Slug != workspaceSlug {
		return zero, errors.New("workspace binding mismatch")
	}

	if time.Now().Unix() > outer.ExpiresAt {
		return zero, errors.New("session expired")
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(encodedInner)
	if err != nil {
		return zero, fmt.Errorf("inner not base64url: %w", err)
	}

	innerHash := sha256.Sum256(ciphertext)
	expectedHash := base64.RawURLEncoding.EncodeToString(innerHash[:])
	if outer.InnerHash != expectedHash {
		return zero, errors.New("inner hash mismatch")
	}

	encKey, err := deriveKey(secret, workspaceSlug)
	if err != nil {
		return zero, fmt.Errorf("derive key: %w", err)
	}

	plaintext, err := decryptAESGCM(encKey, ciphertext)
	if err != nil {
		return zero, fmt.Errorf("decrypt inner: %w", err)
	}

	var inner innerPayload
	if err := json.Unmarshal(plaintext, &inner); err != nil {
		return zero, fmt.Errorf("inner not JSON: %w", err)
	}

	return Session(inner), nil
}

// deriveKey uses HKDF-SHA256 to produce a 32-byte AES key from the
// master secret and workspace slug (domain separation). The salt
// includes a fixed prefix so the same slug in different environments
// (dev/staging/prod) produces different keys even if the master secret
// is accidentally shared.
func deriveKey(secret []byte, workspaceSlug string) ([]byte, error) {
	hk := hkdf.New(sha256.New, secret, []byte("lecrm-"+workspaceSlug), []byte(hkdfInfoKey))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hk, key); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptAESGCM(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// Prepend a 4-byte big-endian length of plaintext for extra validation
	// during decryption (belt-and-suspenders).
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(plaintext)))
	aad := lenBuf
	ciphertext := gcm.Seal(nonce, nonce, plaintext, aad)
	return append(aad, ciphertext...), nil
}

func decryptAESGCM(key, data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, errors.New("ciphertext too short")
	}
	aad := data[:4]
	rest := data[4:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(rest) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short for nonce")
	}
	nonce := rest[:gcm.NonceSize()]
	ciphertext := rest[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, err
	}
	expectedLen := binary.BigEndian.Uint32(aad)
	if uint32(len(plaintext)) != expectedLen {
		return nil, errors.New("plaintext length mismatch")
	}
	return plaintext, nil
}
