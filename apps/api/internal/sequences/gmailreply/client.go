package gmailreply

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrHistoryGap is returned by HistoryClient.MessagesSince when the requested
// startHistoryId is older than Gmail retains (Gmail answers 404). The mailbox
// has changed too much since the cursor; the worker recovers by re-baselining
// (re-watch → store the fresh historyId) rather than crash-looping. Some
// messages in the gap may go uncorrelated — that is the documented trade-off of
// a lapsed cursor (ADR-004 rev 2 §4, "handle history-gap (full re-sync) safely").
var ErrHistoryGap = errors.New("gmailreply: gmail history gap (cursor too old)")

// HistoryClient is one mailbox's Gmail surface, scoped to a single user's
// granted OAuth token. The production implementation (googleClient) wraps
// google.golang.org/api/gmail/v1; tests substitute a fake.
type HistoryClient interface {
	// MessagesSince returns the new INBOX messages added since startHistoryId
	// (label-filtered, reduced to InboundMessage) and the mailbox's latest
	// history id to persist as the new cursor. ErrHistoryGap signals the cursor
	// is too old; any other error is retryable.
	MessagesSince(ctx context.Context, startHistoryID uint64) (msgs []InboundMessage, newHistoryID uint64, err error)

	// Watch (re)registers the Gmail users.watch() Pub/Sub subscription for this
	// mailbox and returns the baseline history id and expiry. Used by the daily
	// watch_renew job and by re-baselining after an ErrHistoryGap.
	Watch(ctx context.Context) (historyID uint64, expiry time.Time, err error)
}

// ClientFactory builds a HistoryClient for a given mailbox. The production
// implementation loads the user's SOPS-stored refresh token (ADR-007 §2),
// exchanges it for an access token (golang.org/x/oauth2/google), and returns a
// gmail/v1-backed client. Kept as a seam so the worker is testable without
// secrets or network.
type ClientFactory interface {
	Client(ctx context.Context, workspaceID, userID uuid.UUID) (HistoryClient, error)
}

// Classification is the OOO-classifier verdict for one inbound reply
// (ADR-004 rev 2 §5). It decides whether a matched reply transitions the
// enrollment to reply_received (a genuine human reply, terminal in v1) or
// ooo_detected (an out-of-office auto-reply that should resume at OOOReturnsAt).
type Classification struct {
	// Category is the classifier label recorded in the audit payload
	// (e.g. "reply", "ooo", "auto_reply"). Free-form; the classifier owns it.
	Category string
	// Confidence is the classifier's confidence in [0,1], for the audit payload.
	Confidence float64
	// IsOOO routes the transition: true → ooo_detected, false → reply_received.
	IsOOO bool
	// OOOReturnsAt, when IsOOO and non-nil, schedules the enrollment to resume
	// (sets ooo_returns_at + next_action_at). Nil OOO means "returns unknown".
	OOOReturnsAt *time.Time
}

// Classifier decides reply_received vs ooo_detected for a matched inbound
// message. The real rules+Haiku implementation is the OOO classifier tasket
// (order:7, 20260614-154815-a81e); this package depends only on the interface.
type Classifier interface {
	Classify(ctx context.Context, msg InboundMessage) (Classification, error)
}

// DefaultClassifier treats every matched reply as a genuine human reply
// (→ reply_received). It is the safe placeholder until the OOO classifier
// (order:7) is wired: misclassifying an OOO as a real reply ends the sequence
// (no further sends), which is conservative — it never keeps emailing someone
// who already engaged.
type DefaultClassifier struct{}

// Classify always returns a non-OOO "reply" verdict.
func (DefaultClassifier) Classify(context.Context, InboundMessage) (Classification, error) {
	return Classification{Category: "reply", Confidence: 1.0, IsOOO: false}, nil
}

// Compile-time proof the default satisfies the seam.
var _ Classifier = DefaultClassifier{}
