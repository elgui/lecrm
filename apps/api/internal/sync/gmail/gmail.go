// Package gmail implements the sync.Provider interface for Google Gmail.
//
// v0 scope: read-only inbound import of Gmail threads, associated with
// leCRM contacts by email address matching. Write-back (labels, stars,
// draft composition) is deferred to v1+.
//
// This is a stub — Pull/Match/ValidateCredentials document the contract
// but panic on invocation. Implementation lands when the Google OAuth
// surface (tasket bf09) provides token exchange and refresh.
package gmail

import (
	"context"

	"github.com/gbconsult/lecrm/apps/api/internal/sync"
)

var _ sync.Provider = (*Provider)(nil)

// Provider implements sync.Provider for Gmail thread import.
type Provider struct{}

// New creates a Gmail sync provider.
func New() *Provider { return &Provider{} }

func (p *Provider) ID() sync.ProviderID   { return sync.ProviderGmail }
func (p *Provider) Direction() sync.SyncDirection { return sync.Inbound }

// Pull fetches Gmail threads modified since the sync cursor position.
//
// Implementation plan (when OAuth lands):
//  1. Decode conn.SyncCursor as Gmail history ID (uint64).
//  2. Call Gmail API users.history.list with startHistoryId.
//  3. For each thread touched: fetch thread metadata + participants.
//  4. Normalize each thread into an InboundRecord:
//     - ExternalID: Gmail thread ID
//     - EntityType: "thread" (stored via metadata engine as object)
//     - Fields: subject, snippet, participant emails, date, labels
//     - MatchHint: Strategy="email", Value=<first participant email matching a contact>
//  5. Return PullResult with records and new history ID as cursor.
//
// Rate limiting: Gmail API quota is 250 units/second per user.
// threads.get = 10 units; history.list = 2 units. A sync batch of
// 100 threads costs ~1020 units ≈ 4s at quota ceiling. Backoff on
// 429 with exponential retry (1s, 2s, 4s, max 3 attempts).
func (p *Provider) Pull(_ context.Context, _ *sync.Connection) (*sync.PullResult, error) {
	panic("gmail: Pull not implemented — waiting for OAuth surface (tasket bf09)")
}

// Match resolves a Gmail thread to a leCRM contact by email address.
//
// Implementation plan:
//  1. Extract participant email addresses from rec.Fields["participants"].
//  2. For each email, query contacts table WHERE email = $1.
//  3. Return the first match with Confidence=1.0 (exact email match).
//  4. If no exact match, try domain matching against companies
//     (Confidence=0.6, below auto-link threshold — surfaces to user).
func (p *Provider) Match(_ context.Context, _ *sync.Connection, _ sync.InboundRecord) (*sync.EntityMatch, error) {
	panic("gmail: Match not implemented — waiting for OAuth surface (tasket bf09)")
}

// ValidateCredentials checks that the stored OAuth2 token is still valid.
//
// Implementation plan:
//  1. Decode credentials as OAuth2 token (access_token, refresh_token, expiry).
//  2. If expired: attempt refresh via Google token endpoint.
//  3. If refresh succeeds: update stored credentials, return nil.
//  4. If refresh fails (revoked/deleted): return error (engine sets status=Revoked).
func (p *Provider) ValidateCredentials(_ context.Context, _ *sync.Connection) error {
	panic("gmail: ValidateCredentials not implemented — waiting for OAuth surface (tasket bf09)")
}
