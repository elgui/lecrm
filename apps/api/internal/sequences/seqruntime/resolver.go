package seqruntime

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences/gmailreply"
)

// ConnectionResolver implements gmailreply.ConnectionResolver against the
// core-schema cross-workspace index (core.gmail_mailbox_index, migration 0027).
// It runs on the main lecrm_api pool (which holds the grant on that table), with
// no workspace scoping — the whole point of the index is to resolve a mailbox to
// a workspace BEFORE any workspace context exists.
type ConnectionResolver struct {
	Pool *pgxpool.Pool
}

// ResolveByEmail maps a Gmail push notification's emailAddress to the owning
// (workspace, user). A miss returns gmailreply.ErrNoConnection so the handler
// acks the delivery (2xx) instead of making Pub/Sub retry a mailbox we cannot act on.
func (r *ConnectionResolver) ResolveByEmail(ctx context.Context, email string) (gmailreply.Target, error) {
	var t gmailreply.Target
	err := r.Pool.QueryRow(ctx,
		`SELECT workspace_id, user_id FROM core.gmail_mailbox_index WHERE email_address = $1`,
		email,
	).Scan(&t.WorkspaceID, &t.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return gmailreply.Target{}, gmailreply.ErrNoConnection
	}
	if err != nil {
		return gmailreply.Target{}, fmt.Errorf("seqruntime: resolve mailbox %q: %w", email, err)
	}
	t.EmailAddress = email
	return t, nil
}

// Compile-time proof ConnectionResolver satisfies the push handler's seam.
var _ gmailreply.ConnectionResolver = (*ConnectionResolver)(nil)
