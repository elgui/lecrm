package seqruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func writeManifest(t *testing.T, dir string, ws, user uuid.UUID, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ws.String()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ws.String(), user.String()+".yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileCredentialStore_Success(t *testing.T) {
	ws, user := uuid.New(), uuid.New()
	dir := t.TempDir()
	writeManifest(t, dir, ws, user,
		"email_address: rep@example.com\n"+
			"oauth_refresh_token: 1//refresh-xyz\n"+
			"oauth_scopes: \"https://www.googleapis.com/auth/gmail.readonly\"\n")

	s := &FileCredentialStore{Dir: dir, ClientID: "cid", ClientSecret: "csecret"}
	creds, err := s.GmailCredentials(context.Background(), ws, user)
	if err != nil {
		t.Fatalf("GmailCredentials: %v", err)
	}
	if creds.RefreshToken != "1//refresh-xyz" {
		t.Errorf("RefreshToken = %q, want 1//refresh-xyz", creds.RefreshToken)
	}
	// client id/secret come from config, not the per-user manifest.
	if creds.ClientID != "cid" || creds.ClientSecret != "csecret" {
		t.Errorf("client creds = (%q,%q), want (cid,csecret)", creds.ClientID, creds.ClientSecret)
	}
}

func TestFileCredentialStore_Errors(t *testing.T) {
	ws, user := uuid.New(), uuid.New()

	t.Run("unconfigured dir", func(t *testing.T) {
		s := &FileCredentialStore{}
		if _, err := s.GmailCredentials(context.Background(), ws, user); err == nil {
			t.Fatal("want error for empty Dir")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		s := &FileCredentialStore{Dir: t.TempDir(), ClientID: "c", ClientSecret: "s"}
		if _, err := s.GmailCredentials(context.Background(), ws, user); err == nil {
			t.Fatal("want error for missing manifest")
		}
	})

	t.Run("empty refresh token", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, ws, user, "email_address: a@b.c\noauth_refresh_token: \"\"\n")
		s := &FileCredentialStore{Dir: dir, ClientID: "c", ClientSecret: "s"}
		if _, err := s.GmailCredentials(context.Background(), ws, user); err == nil {
			t.Fatal("want error for empty refresh token")
		}
	})

	t.Run("malformed yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeManifest(t, dir, ws, user, "this: : : not yaml\n  - broken")
		s := &FileCredentialStore{Dir: dir, ClientID: "c", ClientSecret: "s"}
		if _, err := s.GmailCredentials(context.Background(), ws, user); err == nil {
			t.Fatal("want error for malformed yaml")
		}
	})
}
