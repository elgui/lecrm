// Package config loads runtime configuration from environment variables
// and validates that everything the server needs to come up is present.
//
// Following ADR-009 §1: no struct tags, no config framework — explicit
// env reads keep the binary's startup surface small and auditable.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the resolved runtime configuration for lecrm-api.
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	SessionSecret   []byte
	CookieDomainTLD string // e.g. "lecrm.fr" or "lecrm.test"; per-workspace subdomain is prepended at cookie time
	CookieSecure    bool   // false in dev (no TLS); true in prod

	// CubeJWTSecret signs embed tokens for the Cube.dev container
	// (ADR-009 §9). Must match CUBEJS_API_SECRET in deploy/compose/cube.yml.
	// Empty disables POST /v1/reports/embed-token (handler 503s).
	CubeJWTSecret []byte

	OIDC OIDCConfig
}

// OIDCConfig configures the relying-party against the v0 Authentik IDP
// (ADR-009 §7.1). Identity is keyed on (issuer, sub) — never raw sub.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	CallbackPath string
}

// Load reads configuration from the process environment and returns a
// validated Config. It returns the FIRST validation error encountered;
// callers should treat any error as fatal at startup.
func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:        envOr("LECRM_HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("LECRM_DATABASE_URL"),
		CookieDomainTLD: envOr("LECRM_COOKIE_DOMAIN_TLD", "lecrm.fr"),
		CookieSecure:    envBool("LECRM_COOKIE_SECURE", true),
		OIDC: OIDCConfig{
			Issuer:       os.Getenv("LECRM_OIDC_ISSUER"),
			ClientID:     os.Getenv("LECRM_OIDC_CLIENT_ID"),
			ClientSecret: os.Getenv("LECRM_OIDC_CLIENT_SECRET"),
			Scopes:       splitNonEmpty(envOr("LECRM_OIDC_SCOPES", "openid profile email"), " "),
			CallbackPath: envOr("LECRM_OIDC_CALLBACK_PATH", "/auth/callback"),
		},
	}

	secret := os.Getenv("LECRM_SESSION_SECRET")
	if len(secret) < 32 {
		return nil, errors.New("LECRM_SESSION_SECRET must be at least 32 characters")
	}
	c.SessionSecret = []byte(secret)

	// Optional — empty disables the reports embed-token endpoint. When
	// set, must satisfy the same 32-byte minimum so HS256 signing has a
	// safe key length.
	if cube := os.Getenv("LECRM_CUBE_JWT_SECRET"); cube != "" {
		if len(cube) < 32 {
			return nil, errors.New("LECRM_CUBE_JWT_SECRET must be at least 32 characters when set")
		}
		c.CubeJWTSecret = []byte(cube)
	}

	if c.DatabaseURL == "" {
		return nil, errors.New("LECRM_DATABASE_URL is required")
	}
	if _, err := url.Parse(c.DatabaseURL); err != nil {
		return nil, fmt.Errorf("LECRM_DATABASE_URL invalid: %w", err)
	}

	if c.OIDC.Issuer == "" || c.OIDC.ClientID == "" || c.OIDC.ClientSecret == "" {
		return nil, errors.New("LECRM_OIDC_{ISSUER,CLIENT_ID,CLIENT_SECRET} are required")
	}
	if _, err := url.Parse(c.OIDC.Issuer); err != nil {
		return nil, fmt.Errorf("LECRM_OIDC_ISSUER invalid: %w", err)
	}

	if strings.HasPrefix(c.CookieDomainTLD, ".") || strings.Contains(c.CookieDomainTLD, "*") {
		return nil, fmt.Errorf("LECRM_COOKIE_DOMAIN_TLD must be a bare hostname (got %q)", c.CookieDomainTLD)
	}

	return c, nil
}

// ShutdownTimeout is the upper bound on graceful shutdown.
func (c *Config) ShutdownTimeout() time.Duration { return 10 * time.Second }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func splitNonEmpty(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
