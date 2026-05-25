package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time check: RiverAdapter implements JobRunner.
var _ JobRunner = (*RiverAdapter)(nil)

// RiverAdapter implements JobRunner backed by River
// (github.com/riverqueue/river). When River is added as a dependency,
// the Start/Stop methods wire up River's client lifecycle, and Enqueue
// calls river.Client.Insert. The adapter wraps all workspace-scoped job
// execution with advisory locks via withSafeExec.
type RiverAdapter struct {
	pool     *pgxpool.Pool
	resolver WorkspaceResolver
	creds    CredentialResolver
	logger   *slog.Logger
}

// NewRiverAdapter creates a River-backed job runner. The pool is the
// control-plane connection pool used for job metadata; workspace-scoped
// connections are opened per-job via RunWorkspaceJob.
func NewRiverAdapter(
	pool *pgxpool.Pool,
	resolver WorkspaceResolver,
	creds CredentialResolver,
	logger *slog.Logger,
) *RiverAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &RiverAdapter{
		pool:     pool,
		resolver: resolver,
		creds:    creds,
		logger:   logger,
	}
}

// Enqueue schedules a job for later execution. When River is added as a
// dependency, this becomes river.Client.Insert(ctx, riverJobArgs(job)).
func (ra *RiverAdapter) Enqueue(ctx context.Context, job Job) error {
	if job.WorkspaceID() == (uuid.UUID{}) {
		return fmt.Errorf("river: enqueue: job %q has nil workspace ID", job.Kind())
	}
	ra.logger.InfoContext(ctx, "job enqueued",
		slog.String("kind", job.Kind()),
		slog.String("workspace_id", job.WorkspaceID().String()),
	)
	return nil
}

// Start begins processing enqueued jobs. When River is added, this
// calls river.Client.Start(ctx).
func (ra *RiverAdapter) Start(ctx context.Context) error {
	ra.logger.InfoContext(ctx, "river adapter: started")
	return nil
}

// Stop gracefully shuts down the job processor. When River is added,
// this calls river.Client.Stop(ctx).
func (ra *RiverAdapter) Stop(ctx context.Context) error {
	ra.logger.InfoContext(ctx, "river adapter: stopped")
	return nil
}

// Pool returns the control-plane pool for callers that need to compose
// RunWorkspaceJob with the adapter's resolvers.
func (ra *RiverAdapter) Pool() *pgxpool.Pool { return ra.pool }

// Resolver returns the workspace resolver for callers that need to
// compose RunWorkspaceJob with the adapter.
func (ra *RiverAdapter) Resolver() WorkspaceResolver { return ra.resolver }

// Creds returns the credential resolver for callers that need to
// compose RunWorkspaceJob with the adapter.
func (ra *RiverAdapter) Creds() CredentialResolver { return ra.creds }
