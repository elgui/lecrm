package seqruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences/gmailreply"
)

// FileCredentialStore implements gmailreply.CredentialStore by reading the
// per-(workspace,user) Gmail OAuth manifest that the deploy step renders from
// SOPS to a gitignored plaintext file (decision: decrypt-at-deploy, not runtime
// SOPS in Go — mirrors the Brevo-secret-via-env pattern). Layout:
//
//	<Dir>/<workspace_id>/<user_id>.yaml
//
// The non-secret OAuth client_id and the client_secret come from config (a
// single staging OAuth client); only the long-lived refresh token is per-user
// and lives in the rendered manifest. Manifest schema matches
// secrets/oauth/gmail/_template/secrets.yaml.template.
type FileCredentialStore struct {
	// Dir is the root of the rendered manifests (LECRM_GMAIL_CREDS_DIR).
	Dir string
	// ClientID / ClientSecret are the OAuth web-client credentials shared by the
	// deployment (non-secret id + the SOPS-stored secret, surfaced via config).
	ClientID     string
	ClientSecret string
}

// manifest is the rendered plaintext shape (subset of the committed template;
// email_address/oauth_scopes are self-describing fields we don't strictly need
// at runtime but parse for completeness).
type manifest struct {
	EmailAddress      string `yaml:"email_address"`
	OAuthRefreshToken string `yaml:"oauth_refresh_token"`
	OAuthScopes       string `yaml:"oauth_scopes"`
}

// GmailCredentials loads workspaceID/userID's refresh token from the rendered
// manifest and assembles it with the configured client id/secret.
func (s *FileCredentialStore) GmailCredentials(_ context.Context, workspaceID, userID uuid.UUID) (gmailreply.Credentials, error) {
	if s.Dir == "" {
		return gmailreply.Credentials{}, errors.New("seqruntime: gmail creds dir not configured")
	}
	path := filepath.Join(s.Dir, workspaceID.String(), userID.String()+".yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return gmailreply.Credentials{}, fmt.Errorf("seqruntime: read gmail manifest %s: %w", path, err)
	}
	var m manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return gmailreply.Credentials{}, fmt.Errorf("seqruntime: parse gmail manifest %s: %w", path, err)
	}
	if m.OAuthRefreshToken == "" {
		return gmailreply.Credentials{}, fmt.Errorf("seqruntime: gmail manifest %s has empty oauth_refresh_token", path)
	}
	return gmailreply.Credentials{
		ClientID:     s.ClientID,
		ClientSecret: s.ClientSecret,
		RefreshToken: m.OAuthRefreshToken,
	}, nil
}

// Compile-time proof FileCredentialStore satisfies the client factory's seam.
var _ gmailreply.CredentialStore = (*FileCredentialStore)(nil)
