package seqruntime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
	"github.com/gbconsult/lecrm/apps/api/internal/sequences/gmailreply"
)

// ManagerConfig configures the per-workspace river runtime.
type ManagerConfig struct {
	// Pool is the shared lecrm_api pool the river clients run on. It must be
	// sized for the workspace count (each started client holds a few background
	// connections) plus request traffic.
	Pool *pgxpool.Pool
	// Acquirer opens workspace-scoped txs for the workers (SearchPathAcquirer).
	Acquirer sequences.WorkspaceTxAcquirer
	// GmailDeps are the assembled Gmail-reply worker dependencies (client factory,
	// classifier, logger). Acquirer is set from Acquirer above if unset.
	GmailDeps gmailreply.Deps
	// FoundationHandlers are the send-path handler bodies. Empty for the
	// reply-detection milestone — those workers are registered (so leftover or
	// future send jobs have a worker) but return ErrHandlerNotWired until the
	// send path is wired.
	FoundationHandlers sequences.Handlers
	Logger             *slog.Logger
}

// Manager owns one river.Client per workspace, each pinned to that workspace's
// river_<hex> schema, running the shared worker set + the daily watch-renew.
type Manager struct {
	cfg     ManagerConfig
	logger  *slog.Logger
	mu      sync.RWMutex
	clients map[uuid.UUID]*river.Client[pgx.Tx]
}

// NewManager builds a Manager. Call Start to bring the clients up.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.GmailDeps.Acquirer == nil {
		cfg.GmailDeps.Acquirer = cfg.Acquirer
	}
	return &Manager{
		cfg:     cfg,
		logger:  cfg.Logger,
		clients: make(map[uuid.UUID]*river.Client[pgx.Tx]),
	}
}

// buildWorkers builds the worker registry shared across every workspace client
// (river.Workers is a type registry; one bundle serves all clients).
func (m *Manager) buildWorkers() *river.Workers {
	workers := river.NewWorkers()
	sequences.RegisterWorkers(workers, m.cfg.Acquirer, m.cfg.FoundationHandlers)
	gmailreply.RegisterWorkers(workers, m.cfg.GmailDeps)
	return workers
}

// Start lists every workspace and starts a river client bound to its river_<hex>
// schema. A workspace whose river tables are missing (not yet `lecrm-migrate
// river-setup`) is logged and skipped, so one un-migrated workspace does not
// take the runtime down for the healthy ones.
func (m *Manager) Start(ctx context.Context) error {
	ids, err := m.listWorkspaces(ctx)
	if err != nil {
		return err
	}
	workers := m.buildWorkers()

	started := 0
	for _, id := range ids {
		cfg := sequences.WorkspaceRiverConfig(id, workers, 0)
		cfg.PeriodicJobs = []*river.PeriodicJob{gmailreply.PeriodicWatchRenew(id)}

		client, err := river.NewClient(riverpgxv5.New(m.cfg.Pool), cfg)
		if err != nil {
			m.logger.ErrorContext(ctx, "seqruntime: new client failed",
				"workspace_id", id.String(), "err", err)
			continue
		}
		if err := client.Start(ctx); err != nil {
			m.logger.ErrorContext(ctx, "seqruntime: client start failed (river tables missing? run lecrm-migrate river-setup)",
				"workspace_id", id.String(), "err", err)
			continue
		}
		m.mu.Lock()
		m.clients[id] = client
		m.mu.Unlock()
		started++
	}
	m.logger.InfoContext(ctx, "sequences river runtime started",
		"workspaces_total", len(ids), "clients_started", started)
	return nil
}

func (m *Manager) listWorkspaces(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := m.cfg.Pool.Query(ctx, "SELECT id FROM core.workspaces")
	if err != nil {
		return nil, fmt.Errorf("seqruntime: list workspaces: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("seqruntime: scan workspace id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Client returns the running river client for a workspace (used by the push
// enqueuer to insert poll_mailbox into the right per-tenant queue).
func (m *Manager) Client(workspaceID uuid.UUID) (*river.Client[pgx.Tx], bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[workspaceID]
	return c, ok
}

// Stop gracefully stops every running client. Safe to call once.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for id, c := range m.clients {
		if err := c.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(m.clients, id)
	}
	return firstErr
}

// Enqueuer implements gmailreply.MailboxPollEnqueuer by inserting into the
// resolved workspace's river client. A push for a workspace whose runtime is
// not up returns an error → the handler 500s and Pub/Sub retries (correct: the
// notification is real, the runtime is just not ready).
type Enqueuer struct {
	Manager *Manager
}

// EnqueuePollMailbox inserts a poll_mailbox job into the workspace's queue.
func (e *Enqueuer) EnqueuePollMailbox(ctx context.Context, args gmailreply.PollMailboxArgs) error {
	client, ok := e.Manager.Client(args.WorkspaceID)
	if !ok {
		return fmt.Errorf("seqruntime: no river client for workspace %s", args.WorkspaceID)
	}
	if _, err := client.Insert(ctx, args, nil); err != nil {
		return fmt.Errorf("seqruntime: insert poll_mailbox: %w", err)
	}
	return nil
}

// Compile-time proof Enqueuer satisfies the push handler's enqueue seam.
var _ gmailreply.MailboxPollEnqueuer = (*Enqueuer)(nil)
