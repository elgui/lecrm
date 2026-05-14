package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// Provider is the configured zitadel/oidc relying-party plus the
// session-secret used to sign the short-lived OAuth state cookie.
//
// One Provider exists per-process; it is shared across all workspaces
// because the IDP (and therefore the OIDC issuer) is global at v0.
// Workspaces are distinguished by the redirect URI and the resulting
// session-cookie Domain — both per-request inputs. The redirect URI
// passed to NewRelyingPartyOIDC is a placeholder; callers MUST override
// it per-request via the redirectURI argument to BuildAuthURL and
// Exchange so the value reaching Authentik exactly matches the
// workspace subdomain.
type Provider struct {
	RP           rp.RelyingParty
	StateSecret  []byte // separate from session secret so rotation can be staged
	Issuer       string
	Scopes       []string
	CallbackPath string
}

// NewProvider builds the RP by hitting the IDP's well-known discovery
// endpoint. It blocks for one HTTP round-trip; call once at startup.
func NewProvider(ctx context.Context, issuer, clientID, clientSecret, redirectURI string, scopes []string, stateSecret []byte) (*Provider, error) {
	relying, err := rp.NewRelyingPartyOIDC(ctx, issuer, clientID, clientSecret, redirectURI, scopes)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery against %q failed: %w", issuer, err)
	}
	return &Provider{
		RP:          relying,
		StateSecret: stateSecret,
		Issuer:      issuer,
		Scopes:      scopes,
	}, nil
}

// AuthRequest is the per-login transient state stored in the workspace-
// scoped state cookie between /auth/login and /auth/callback.
type AuthRequest struct {
	State              string    `json:"st"`
	CodeVerifier       string    `json:"cv"`
	WorkspaceSubdomain string    `json:"ws"`
	RedirectURI        string    `json:"ru"`
	IssuedAt           time.Time `json:"ia"`
}

// BuildAuthURL generates a fresh state nonce and PKCE code verifier,
// returns the AuthRequest to be stamped into the state cookie and the
// authorization URL the browser should redirect to. redirectURI MUST be
// the workspace-specific callback URL — the IdP rejects mismatches.
//
// PKCE is mandatory per OAuth 2.1 / OIDC best practice and protects
// against authorization-code interception for public clients. Even
// though our client is confidential, PKCE adds defence in depth at
// negligible cost.
func (p *Provider) BuildAuthURL(workspaceSubdomain, redirectURI string) (AuthRequest, string, error) {
	state, err := randomString(32)
	if err != nil {
		return AuthRequest{}, "", err
	}
	verifier, err := randomString(64)
	if err != nil {
		return AuthRequest{}, "", err
	}
	challenge := oidc.NewSHACodeChallenge(verifier)
	// rp.WithURLParam returns URLParamOpt; AuthURL takes AuthURLOpt.
	// They share the same underlying signature so a direct cast is the
	// idiomatic bridge (the library does this internally in AuthURLHandler).
	authURL := rp.AuthURL(state, p.RP,
		rp.WithCodeChallenge(challenge),
		rp.AuthURLOpt(rp.WithURLParam("redirect_uri", redirectURI)),
	)
	return AuthRequest{
		State:              state,
		CodeVerifier:       verifier,
		WorkspaceSubdomain: workspaceSubdomain,
		RedirectURI:        redirectURI,
		IssuedAt:           time.Now(),
	}, authURL, nil
}

// Exchange exchanges the authorization code for tokens, verifies the
// ID token, and returns the parsed claims. The (Issuer, Subject) pair
// is the canonical identity tuple per ADR-009 §7.1. redirectURI MUST
// match the URI sent in the original authorization request.
func (p *Provider) Exchange(ctx context.Context, code, codeVerifier, redirectURI string) (*oidc.IDTokenClaims, error) {
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, p.RP,
		rp.WithCodeVerifier(codeVerifier),
		rp.CodeExchangeOpt(rp.WithURLParam("redirect_uri", redirectURI)),
	)
	if err != nil {
		return nil, fmt.Errorf("code exchange failed: %w", err)
	}
	if tokens == nil || tokens.IDTokenClaims == nil {
		return nil, errors.New("id_token missing from token response")
	}
	if tokens.IDTokenClaims.Issuer == "" || tokens.IDTokenClaims.Subject == "" {
		return nil, errors.New("id_token is missing issuer or subject")
	}
	return tokens.IDTokenClaims, nil
}

// randomString returns a base64url-encoded random string of approximately
// the requested byte length. Used for state nonces and PKCE verifiers.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
