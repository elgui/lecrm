package gmailreply

import (
	"errors"
	"testing"

	"google.golang.org/api/idtoken"
)

func payload(issuer, email string, verified any) *idtoken.Payload {
	return &idtoken.Payload{
		Issuer: issuer,
		Claims: map[string]interface{}{
			"email":          email,
			"email_verified": verified,
		},
	}
}

func TestVerifyPayloadClaims_Valid(t *testing.T) {
	for _, iss := range []string{"https://accounts.google.com", "accounts.google.com"} {
		got, err := verifyPayloadClaims(payload(iss, "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com", true))
		if err != nil {
			t.Fatalf("issuer %q: unexpected error %v", iss, err)
		}
		if got != "gmail-push-invoker@lecrm-prod.iam.gserviceaccount.com" {
			t.Errorf("email = %q", got)
		}
	}
}

func TestVerifyPayloadClaims_Rejects(t *testing.T) {
	cases := []struct {
		name string
		p    *idtoken.Payload
	}{
		{"nil payload", nil},
		{"wrong issuer", payload("https://evil.example.com", "sa@x", true)},
		{"missing email", payload("https://accounts.google.com", "", true)},
		{"email not verified", payload("https://accounts.google.com", "sa@x", false)},
		{"email_verified wrong type", payload("https://accounts.google.com", "sa@x", "true")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifyPayloadClaims(tc.p); err == nil {
				t.Fatalf("expected error, got nil")
			} else if !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("error %v is not ErrInvalidToken", err)
			}
		})
	}
}
