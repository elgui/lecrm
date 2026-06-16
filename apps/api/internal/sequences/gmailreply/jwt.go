package gmailreply

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/api/idtoken"
)

// ErrInvalidToken is returned when the Pub/Sub push OIDC token fails
// validation (bad signature, wrong audience, expired, wrong issuer, or missing
// a verified email claim). The push handler maps it to 401 — satisfying the
// "rejects requests without a valid Google-signed JWT" requirement (ADR-004
// rev 2 §4).
var ErrInvalidToken = errors.New("gmailreply: invalid push token")

// googleIssuers are the two issuer spellings Google emits for OIDC id tokens.
var googleIssuers = map[string]struct{}{
	"https://accounts.google.com": {},
	"accounts.google.com":         {},
}

// TokenValidator verifies the Google-signed OIDC JWT that Pub/Sub attaches to
// each push (the `Authorization: Bearer <jwt>` header). On success it returns
// the verified `email` claim — the push-auth service account the subscription
// was created with — which the handler additionally checks against the expected
// account. The seam keeps the handler unit-testable without live Google certs.
type TokenValidator interface {
	Validate(ctx context.Context, rawToken, audience string) (email string, err error)
}

// GoogleTokenValidator validates the token against Google's public certs via
// google.golang.org/api/idtoken (signature, audience, expiry) and then enforces
// the issuer and a verified email claim. It is a thin, dependency-backed
// wrapper; the verifiable claim logic lives in verifyPayloadClaims so it can be
// exercised in unit tests with synthetic payloads.
type GoogleTokenValidator struct{}

// Validate implements TokenValidator using idtoken.Validate.
func (GoogleTokenValidator) Validate(ctx context.Context, rawToken, audience string) (string, error) {
	payload, err := idtoken.Validate(ctx, rawToken, audience)
	if err != nil {
		// idtoken already checked signature/aud/exp; any failure is a reject.
		return "", fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	return verifyPayloadClaims(payload)
}

// verifyPayloadClaims enforces the application-level checks idtoken.Validate
// does not: the issuer must be Google, and the token must carry a verified,
// non-empty email claim (the push-auth service account). It returns that email
// on success. Pure over its input, so it is unit-tested directly.
func verifyPayloadClaims(p *idtoken.Payload) (string, error) {
	if p == nil {
		return "", fmt.Errorf("%w: nil payload", ErrInvalidToken)
	}
	if _, ok := googleIssuers[p.Issuer]; !ok {
		return "", fmt.Errorf("%w: issuer %q not Google", ErrInvalidToken, p.Issuer)
	}
	email, _ := p.Claims["email"].(string)
	if email == "" {
		return "", fmt.Errorf("%w: missing email claim", ErrInvalidToken)
	}
	// email_verified is a bool in Google id tokens; require it explicitly so a
	// token minted for an unverified identity cannot impersonate the SA.
	if verified, ok := p.Claims["email_verified"].(bool); !ok || !verified {
		return "", fmt.Errorf("%w: email not verified", ErrInvalidToken)
	}
	return email, nil
}

// Compile-time proof the production validator satisfies the seam.
var _ TokenValidator = GoogleTokenValidator{}
