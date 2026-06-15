package gmailreply

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// gmailUserMe is the Gmail API alias for "the authenticated user" — the only
// mailbox a per-user OAuth grant can reach.
const gmailUserMe = "me"

// inboxLabel restricts history + watch to the INBOX, so replies (not sent mail
// or drafts) are what surface (ADR-004 rev 2 §4: labelIds=["INBOX"]).
const inboxLabel = "INBOX"

// Credentials is one user's Gmail OAuth material, assembled from the
// per-workspace client id (non-secret config) and the SOPS-stored client secret
// + refresh token (ADR-007 §2).
type Credentials struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
}

// CredentialStore yields a user's Gmail OAuth credentials. The production
// implementation reads the SOPS-decrypted refresh-token manifest at
// secrets/oauth/gmail/<workspace_id>/<user_id>.enc.yaml plus the per-workspace
// client config. It is a seam so the client factory carries no secret-store
// coupling and tests stay offline.
type CredentialStore interface {
	GmailCredentials(ctx context.Context, workspaceID, userID uuid.UUID) (Credentials, error)
}

// GoogleClientFactory builds gmail/v1-backed HistoryClients. It is the
// production ClientFactory: it exchanges each user's refresh token for access
// tokens (golang.org/x/oauth2/google) and constructs a Gmail service scoped to
// that mailbox.
type GoogleClientFactory struct {
	// Creds resolves per-(workspace,user) OAuth credentials.
	Creds CredentialStore
	// TopicName is the fully-qualified Pub/Sub topic users.watch() publishes to
	// (projects/<project>/topics/gmail-inbox-events). Required for Watch().
	TopicName string
}

// Client implements ClientFactory.
func (f *GoogleClientFactory) Client(ctx context.Context, workspaceID, userID uuid.UUID) (HistoryClient, error) {
	if f.Creds == nil {
		return nil, errors.New("gmailreply: GoogleClientFactory.Creds not configured")
	}
	creds, err := f.Creds.GmailCredentials(ctx, workspaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("gmailreply: load credentials for %s/%s: %w", workspaceID, userID, err)
	}
	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{gmail.GmailReadonlyScope},
	}
	ts := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: creds.RefreshToken})
	svc, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("gmailreply: build gmail service: %w", err)
	}
	return &googleClient{svc: svc, topic: f.TopicName}, nil
}

// googleClient is the gmail/v1-backed HistoryClient for one mailbox.
type googleClient struct {
	svc   *gmail.Service
	topic string
}

// MessagesSince walks Gmail history from startHistoryID (messageAdded on INBOX),
// fetches each new message's correlation headers, and returns them with the
// mailbox's latest history id. A 404 from Gmail means the start id is too old →
// ErrHistoryGap.
func (c *googleClient) MessagesSince(ctx context.Context, startHistoryID uint64) ([]InboundMessage, uint64, error) {
	var (
		msgs   []InboundMessage
		latest = startHistoryID
		seen   = make(map[string]struct{})
	)
	call := c.svc.Users.History.List(gmailUserMe).
		StartHistoryId(startHistoryID).
		HistoryTypes("messageAdded").
		LabelId(inboxLabel)

	err := call.Pages(ctx, func(resp *gmail.ListHistoryResponse) error {
		if resp.HistoryId > latest {
			latest = resp.HistoryId
		}
		for _, h := range resp.History {
			for _, added := range h.MessagesAdded {
				if added.Message == nil || added.Message.Id == "" {
					continue
				}
				if _, dup := seen[added.Message.Id]; dup {
					continue
				}
				seen[added.Message.Id] = struct{}{}
				im, err := c.fetchMessage(ctx, added.Message.Id)
				if err != nil {
					return err
				}
				if im != nil {
					msgs = append(msgs, *im)
				}
			}
		}
		return nil
	})
	if err != nil {
		if isHistoryGap(err) {
			return nil, 0, ErrHistoryGap
		}
		return nil, 0, fmt.Errorf("gmailreply: history.list: %w", err)
	}
	return msgs, latest, nil
}

// fetchMessage reads only the metadata headers needed to thread and classify a
// reply (format=metadata keeps the body — and most PII — out of the response).
func (c *googleClient) fetchMessage(ctx context.Context, id string) (*InboundMessage, error) {
	m, err := c.svc.Users.Messages.Get(gmailUserMe, id).
		Format("metadata").
		MetadataHeaders("Message-Id", "In-Reply-To", "References", "From", "Subject").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("gmailreply: messages.get %s: %w", id, err)
	}
	if m == nil {
		return nil, nil
	}
	im := InboundMessage{GmailMessageID: m.Id, Snippet: m.Snippet}
	if m.Payload != nil {
		for _, hdr := range m.Payload.Headers {
			switch strings.ToLower(hdr.Name) {
			case "message-id":
				im.RFC822MessageID = NormalizeMessageID(hdr.Value)
			case "in-reply-to":
				im.InReplyTo = hdr.Value
			case "references":
				im.References = strings.Fields(hdr.Value)
			case "from":
				im.From = hdr.Value
			case "subject":
				im.Subject = hdr.Value
			}
		}
	}
	return &im, nil
}

// Watch (re)registers users.watch() for this mailbox against the configured
// Pub/Sub topic and returns the baseline history id and expiry.
func (c *googleClient) Watch(ctx context.Context) (uint64, time.Time, error) {
	if c.topic == "" {
		return 0, time.Time{}, errors.New("gmailreply: watch topic not configured")
	}
	resp, err := c.svc.Users.Watch(gmailUserMe, &gmail.WatchRequest{
		TopicName: c.topic,
		LabelIds:  []string{inboxLabel},
	}).Context(ctx).Do()
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("gmailreply: users.watch: %w", err)
	}
	return resp.HistoryId, time.UnixMilli(resp.Expiration), nil
}

// isHistoryGap reports whether err is the Gmail 404 that signals startHistoryId
// is older than Gmail retains (the cursor must be re-baselined).
func isHistoryGap(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == http.StatusNotFound
	}
	return false
}

// Compile-time proofs the production types satisfy the seams.
var (
	_ ClientFactory = (*GoogleClientFactory)(nil)
	_ HistoryClient = (*googleClient)(nil)
)
