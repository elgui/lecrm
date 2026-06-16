package gmailreply

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrNoConnection is returned by a ConnectionResolver when an emailAddress maps
// to no connected mailbox. The push handler treats it as "not ours" — it acks
// the delivery (2xx) so Pub/Sub stops redelivering, and logs it; a 5xx would
// make Pub/Sub retry a notification we can never act on.
var ErrNoConnection = errors.New("gmailreply: no connection for email address")

// Target is the (workspace, user) a Gmail push resolves to, plus the address it
// resolved from. The mailbox-scan job is keyed on the (WorkspaceID, UserID)
// pair.
type Target struct {
	WorkspaceID  uuid.UUID
	UserID       uuid.UUID
	EmailAddress string
}

// ConnectionResolver maps the emailAddress in a Gmail push notification to the
// workspace+user whose connection owns that mailbox. Because a push arrives
// outside any workspace context, this lookup spans workspaces; the production
// implementation backs it with the cross-workspace connection registry (e.g. a
// core-schema email→connection index maintained at OAuth-grant time). It is a
// seam so the push handler is unit-testable without a database.
type ConnectionResolver interface {
	ResolveByEmail(ctx context.Context, email string) (Target, error)
}

// MailboxPollEnqueuer enqueues a sequences.gmail.poll_mailbox job. The
// production implementation inserts into the resolved workspace's per-tenant
// river schema (river_<hex>) via that workspace's river client (the args carry
// WorkspaceID); a test substitutes a recorder. Kept here so the push handler
// depends on the narrow capability, not a concrete river client.
type MailboxPollEnqueuer interface {
	EnqueuePollMailbox(ctx context.Context, args PollMailboxArgs) error
}
