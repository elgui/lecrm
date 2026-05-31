package db

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkspaceResolver maps a workspace UUID to its Postgres role name.
type WorkspaceResolver interface {
	WorkspaceRoleName(ctx context.Context, id uuid.UUID) (string, error)
}

// CredentialResolver returns a Postgres DSN for a given role.
type CredentialResolver interface {
	DSNForRole(ctx context.Context, roleName string) (string, error)
}

// TenantPoolConfig tunes the bounded pool manager.
type TenantPoolConfig struct {
	MaxPools        int
	MaxConnsPerPool int32
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func (c *TenantPoolConfig) applyDefaults() {
	if c.MaxPools <= 0 {
		c.MaxPools = 20
	}
	if c.MaxConnsPerPool <= 0 {
		c.MaxConnsPerPool = 3
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = time.Hour
	}
	if c.ConnMaxIdleTime <= 0 {
		c.ConnMaxIdleTime = 5 * time.Minute
	}
}

// TenantPool manages a bounded set of pgxpool.Pool instances, one per
// workspace role. When the pool count exceeds MaxPools, the least-recently
// used pool is evicted. This bounds total Postgres connections to
// MaxPools × MaxConnsPerPool regardless of tenant count.
//
// All sub-pools use QueryExecModeSimpleProtocol to avoid prepared-statement
// cache bloat and to be compatible with PgBouncer transaction mode.
type TenantPool struct {
	mu       sync.Mutex
	pools    map[string]*pgxpool.Pool // keyed by role name
	lru      *list.List               // front = most recently used
	lruIdx   map[string]*list.Element // role name → list element

	resolver WorkspaceResolver
	creds    CredentialResolver
	config   TenantPoolConfig

	closed bool
}

// NewTenantPool creates a bounded tenant pool manager.
func NewTenantPool(resolver WorkspaceResolver, creds CredentialResolver, cfg TenantPoolConfig) *TenantPool {
	cfg.applyDefaults()
	return &TenantPool{
		pools:    make(map[string]*pgxpool.Pool),
		lru:      list.New(),
		lruIdx:   make(map[string]*list.Element),
		resolver: resolver,
		creds:    creds,
		config:   cfg,
	}
}

// TenantPoolStats reports pool manager state for health checks.
type TenantPoolStats struct {
	ActivePools   int
	MaxPools      int
	TotalAcquired int64
	TotalIdle     int64
	MaxConnsTotal int64
}

// Stats returns a snapshot of pool utilization.
func (tp *TenantPool) Stats() TenantPoolStats {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	s := TenantPoolStats{
		ActivePools: len(tp.pools),
		MaxPools:    tp.config.MaxPools,
	}
	for _, p := range tp.pools {
		stat := p.Stat()
		s.TotalAcquired += int64(stat.AcquiredConns())
		s.TotalIdle += int64(stat.IdleConns())
		s.MaxConnsTotal += int64(stat.MaxConns())
	}
	return s
}

// RunInWorkspace acquires a pooled connection scoped to a workspace, verifies
// the search_path, and executes fn. The connection is returned to the pool
// when fn returns.
//
// This replaces the raw pgx.Connect pattern in RunWorkspaceJob with bounded
// pooled connections.
func (tp *TenantPool) RunInWorkspace(
	ctx context.Context,
	workspaceID uuid.UUID,
	fn func(ctx context.Context, conn *pgxpool.Conn) error,
) error {
	roleName, err := tp.resolver.WorkspaceRoleName(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("tenant pool: resolve workspace %s: %w", workspaceID, err)
	}

	pool, err := tp.getOrCreate(ctx, roleName)
	if err != nil {
		return fmt.Errorf("tenant pool: pool for %s: %w", roleName, err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("tenant pool: acquire conn for %s: %w", roleName, err)
	}
	defer conn.Release()

	var searchPath string
	if err := conn.QueryRow(ctx, "SHOW search_path").Scan(&searchPath); err != nil {
		return fmt.Errorf("tenant pool: verify search_path: %w", err)
	}
	if !strings.Contains(searchPath, roleName) {
		return fmt.Errorf(
			"tenant pool: search_path %q does not contain workspace schema %q; connection mis-scoped",
			searchPath, roleName)
	}

	return fn(ctx, conn)
}

// Close shuts down all managed pools. Safe to call multiple times.
func (tp *TenantPool) Close() {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.closed {
		return
	}
	tp.closed = true

	for name, p := range tp.pools {
		p.Close()
		delete(tp.pools, name)
	}
	tp.lru.Init()
	for k := range tp.lruIdx {
		delete(tp.lruIdx, k)
	}
}

// PoolCount returns the number of active workspace pools. Intended for tests.
func (tp *TenantPool) PoolCount() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return len(tp.pools)
}

func (tp *TenantPool) getOrCreate(ctx context.Context, roleName string) (*pgxpool.Pool, error) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.closed {
		return nil, fmt.Errorf("tenant pool is closed")
	}

	if p, ok := tp.pools[roleName]; ok {
		tp.touch(roleName)
		return p, nil
	}

	if len(tp.pools) >= tp.config.MaxPools {
		tp.evictLRU()
	}

	dsn, err := tp.creds.DSNForRole(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("resolve dsn for %s: %w", roleName, err)
	}

	pool, err := tp.openSubPool(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool for %s: %w", roleName, err)
	}

	tp.pools[roleName] = pool
	elem := tp.lru.PushFront(roleName)
	tp.lruIdx[roleName] = elem

	return pool, nil
}

func (tp *TenantPool) openSubPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	cfg.MaxConns = tp.config.MaxConnsPerPool
	cfg.MaxConnLifetime = tp.config.ConnMaxLifetime
	cfg.MaxConnIdleTime = tp.config.ConnMaxIdleTime

	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	return pgxpool.NewWithConfig(ctx, cfg)
}

func (tp *TenantPool) touch(roleName string) {
	if elem, ok := tp.lruIdx[roleName]; ok {
		tp.lru.MoveToFront(elem)
	}
}

func (tp *TenantPool) evictLRU() {
	back := tp.lru.Back()
	if back == nil {
		return
	}

	roleName, _ := back.Value.(string)
	if p, ok := tp.pools[roleName]; ok {
		p.Close()
		delete(tp.pools, roleName)
	}
	tp.lru.Remove(back)
	delete(tp.lruIdx, roleName)
}
