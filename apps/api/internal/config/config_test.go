package config

import "testing"

// baseEnv sets the minimal env Load() requires so a test can layer Gmail vars
// on top and exercise just the Gmail gating/validation.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LECRM_SESSION_SECRET", "0123456789012345678901234567890123")
	t.Setenv("LECRM_DATABASE_URL", "postgres://localhost:5432/lecrm")
	t.Setenv("LECRM_OIDC_ISSUER", "https://idp.example.com")
	t.Setenv("LECRM_OIDC_CLIENT_ID", "cid")
	t.Setenv("LECRM_OIDC_CLIENT_SECRET", "csecret")
}

func TestLoad_GmailDisabledByDefault(t *testing.T) {
	baseEnv(t)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Gmail.Enabled() {
		t.Error("Gmail should be disabled when no LECRM_GMAIL_* env is set")
	}
}

func TestLoad_GmailEnabledRequiresAudienceAndOAuth(t *testing.T) {
	baseEnv(t)
	// Topic + creds dir flip Enabled() true, but the security-relevant knobs are
	// still missing — Load must reject so a half-configured push route is never
	// exposed.
	t.Setenv("LECRM_GMAIL_PUBSUB_TOPIC", "projects/p/topics/gmail-inbox-events")
	t.Setenv("LECRM_GMAIL_CREDS_DIR", "/run/secrets/gmail")
	if _, err := Load(); err == nil {
		t.Fatal("want error: Gmail enabled without push audience / oauth client")
	}
}

func TestLoad_GmailEnabledValid(t *testing.T) {
	baseEnv(t)
	t.Setenv("LECRM_GMAIL_PUBSUB_TOPIC", "projects/p/topics/gmail-inbox-events")
	t.Setenv("LECRM_GMAIL_CREDS_DIR", "/run/secrets/gmail")
	t.Setenv("LECRM_GMAIL_PUSH_AUDIENCE", "https://demo.lecrm.gbconsult.me/v1/webhooks/gmail/push")
	t.Setenv("LECRM_GMAIL_PUSH_SA", "gmail-push-invoker@proj.iam.gserviceaccount.com")
	t.Setenv("LECRM_GMAIL_OAUTH_CLIENT_ID", "client-id")
	t.Setenv("LECRM_GMAIL_OAUTH_CLIENT_SECRET", "client-secret")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Gmail.Enabled() {
		t.Fatal("Gmail should be enabled")
	}
	if c.Gmail.PushServiceAccount != "gmail-push-invoker@proj.iam.gserviceaccount.com" {
		t.Errorf("PushServiceAccount = %q", c.Gmail.PushServiceAccount)
	}
}
