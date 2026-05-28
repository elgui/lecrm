package auth

// Workspace-scoped Bearer service tokens (ADR-009 §4.1, ADR-011 §6).
//
// Plaintext format:
//
//	lecrm_<workspace_slug>_<base64url(32 random bytes)>
//
// Persistence: argon2id hash only. The plaintext is returned ONCE at
// creation time and never stored. The workspace slug is embedded in
// the token so the middleware can scope the verification lookup to a
// single tenant without an O(N) scan across the whole table.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// TokenPrefix is the constant leading literal on every leCRM service
// token. Tools that scan repos / logs for accidental credential
// disclosure can grep for this prefix.
const TokenPrefix = "lecrm_"

// argon2id parameters — RFC 9106 §4 "second recommended option" for
// memory-constrained environments. Time=1 + memory=64MB + parallelism=4
// gives ~50ms / verify on a modern x86-64 CPU. Tunable upward; we keep
// the current defaults conservative because token verification runs
// on the request hot path.
const (
	argonTime    uint32 = 1
	argonMemory  uint32 = 64 * 1024 // 64 MiB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

// ErrInvalidTokenFormat is returned when a candidate plaintext does
// not match the expected `lecrm_<slug>_<random>` shape.
var ErrInvalidTokenFormat = errors.New("service token: invalid format")

// GenerateServiceToken returns a fresh plaintext token + its argon2id
// hash. The hash format is the canonical PHC string:
//
//	$argon2id$v=19$m=65536,t=1,p=4$<saltB64>$<hashB64>
//
// Callers MUST persist only the returned hash and surface the plaintext
// to the user exactly once.
func GenerateServiceToken(workspaceSlug string) (plaintext string, hash string, err error) {
	if workspaceSlug == "" {
		return "", "", errors.New("service token: workspace slug required")
	}
	// 32 bytes of entropy → 256 bits, far beyond birthday-collision
	// concerns for tokens scoped to a single workspace.
	randBytes := make([]byte, 32)
	if _, err := rand.Read(randBytes); err != nil {
		return "", "", fmt.Errorf("service token: rand read: %w", err)
	}
	suffix := base64.RawURLEncoding.EncodeToString(randBytes)
	plaintext = TokenPrefix + workspaceSlug + "_" + suffix

	hash, err = hashToken(plaintext)
	if err != nil {
		return "", "", err
	}
	return plaintext, hash, nil
}

// WorkspaceSlugFromToken extracts the workspace slug embedded in a
// candidate plaintext token. The function performs structural
// validation only — it never reveals whether the token exists.
//
// Returns ErrInvalidTokenFormat for any token that does not match
// `lecrm_<slug>_<rest>`.
func WorkspaceSlugFromToken(token string) (string, error) {
	if !strings.HasPrefix(token, TokenPrefix) {
		return "", ErrInvalidTokenFormat
	}
	rest := token[len(TokenPrefix):]
	idx := strings.Index(rest, "_")
	if idx <= 0 || idx == len(rest)-1 {
		return "", ErrInvalidTokenFormat
	}
	slug := rest[:idx]
	// Slug shape: alnum + hyphen, 1..63 chars. Matches workspace.subdomainOf.
	for _, c := range slug {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return "", ErrInvalidTokenFormat
		}
	}
	if len(slug) > 63 {
		return "", ErrInvalidTokenFormat
	}
	return slug, nil
}

// VerifyServiceToken constant-time-compares a candidate plaintext
// against a stored hash. Returns nil on match.
func VerifyServiceToken(token, storedHash string) error {
	parsed, err := parseArgonHash(storedHash)
	if err != nil {
		return err
	}
	candidate := argon2.IDKey([]byte(token), parsed.salt, parsed.t, parsed.m, parsed.p, parsed.keyLen)
	if subtle.ConstantTimeCompare(candidate, parsed.hash) != 1 {
		return errors.New("service token: hash mismatch")
	}
	return nil
}

func hashToken(plaintext string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("service token: salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

type argonParsed struct {
	salt   []byte
	hash   []byte
	t      uint32
	m      uint32
	p      uint8
	keyLen uint32
}

func parseArgonHash(s string) (argonParsed, error) {
	var z argonParsed
	parts := strings.Split(s, "$")
	// Expected: "", "argon2id", "v=19", "m=...,t=...,p=...", "<saltB64>", "<hashB64>"
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return z, errors.New("service token: malformed argon2id hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return z, errors.New("service token: missing argon2id version")
	}
	if version != argon2.Version {
		return z, errors.New("service token: unsupported argon2id version")
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return z, errors.New("service token: malformed argon2id params")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return z, errors.New("service token: malformed salt")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return z, errors.New("service token: malformed hash")
	}
	return argonParsed{
		salt: salt, hash: hash, t: t, m: m, p: p,
		keyLen: uint32(len(hash)),
	}, nil
}

// FingerprintToken returns a short hex prefix over the random suffix
// of the plaintext token. Used in audit logs / telemetry where we
// want a stable identifier without leaking the token. NOT a security
// primitive: do not use for authn.
//
// The random suffix is base64url-encoded — its alphabet INCLUDES `_`,
// so we cannot use strings.LastIndex to find the suffix boundary.
// Instead we strip the documented `lecrm_<slug>_` prefix.
func FingerprintToken(token string) string {
	if !strings.HasPrefix(token, TokenPrefix) {
		return ""
	}
	rest := token[len(TokenPrefix):]
	idx := strings.Index(rest, "_")
	if idx <= 0 || idx == len(rest)-1 {
		return ""
	}
	suffix := rest[idx+1:]
	raw, err := base64.RawURLEncoding.DecodeString(suffix)
	if err != nil || len(raw) < 8 {
		return ""
	}
	return hex.EncodeToString(raw[:8])
}
