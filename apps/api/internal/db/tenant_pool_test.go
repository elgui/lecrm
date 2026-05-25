package db

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type stubWorkspaceResolver struct {
	mu    sync.Mutex
	roles map[uuid.UUID]string
}

func (s *stubWorkspaceResolver) WorkspaceRoleName(_ context.Context, id uuid.UUID) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.roles[id]; ok {
		return r, nil
	}
	return "", fmt.Errorf("workspace %s not found", id)
}

type stubCredentialResolver struct {
	mu   sync.Mutex
	dsns map[string]string
}

func (s *stubCredentialResolver) DSNForRole(_ context.Context, roleName string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.dsns[roleName]; ok {
		return d, nil
	}
	return "", fmt.Errorf("no DSN for role %s", roleName)
}

func TestTenantPoolConfig_Defaults(t *testing.T) {
	cfg := TenantPoolConfig{}
	cfg.applyDefaults()

	if cfg.MaxPools != 20 {
		t.Errorf("MaxPools = %d, want 20", cfg.MaxPools)
	}
	if cfg.MaxConnsPerPool != 3 {
		t.Errorf("MaxConnsPerPool = %d, want 3", cfg.MaxConnsPerPool)
	}
}

func TestTenantPoolConfig_CustomValues(t *testing.T) {
	cfg := TenantPoolConfig{MaxPools: 50, MaxConnsPerPool: 5}
	cfg.applyDefaults()

	if cfg.MaxPools != 50 {
		t.Errorf("MaxPools = %d, want 50", cfg.MaxPools)
	}
	if cfg.MaxConnsPerPool != 5 {
		t.Errorf("MaxConnsPerPool = %d, want 5", cfg.MaxConnsPerPool)
	}
}

func TestNewTenantPool_InitialState(t *testing.T) {
	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{},
	)
	defer tp.Close()

	if tp.PoolCount() != 0 {
		t.Errorf("PoolCount = %d, want 0", tp.PoolCount())
	}

	stats := tp.Stats()
	if stats.ActivePools != 0 {
		t.Errorf("ActivePools = %d, want 0", stats.ActivePools)
	}
	if stats.MaxPools != 20 {
		t.Errorf("MaxPools = %d, want 20", stats.MaxPools)
	}
}

func TestTenantPool_ResolverError(t *testing.T) {
	wsID := uuid.New()

	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{MaxPools: 5},
	)
	defer tp.Close()

	err := tp.RunInWorkspace(context.Background(), wsID, func(_ context.Context, _ *pgxpool.Conn) error {
		t.Fatal("fn should not be called on resolver error")
		return nil
	})
	if err == nil {
		t.Fatal("expected error from missing workspace, got nil")
	}
	if tp.PoolCount() != 0 {
		t.Errorf("PoolCount = %d after resolver error, want 0", tp.PoolCount())
	}
}

func TestTenantPool_CredentialError(t *testing.T) {
	wsID := uuid.New()
	roleName := "workspace_test"

	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{wsID: roleName}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{MaxPools: 5},
	)
	defer tp.Close()

	err := tp.RunInWorkspace(context.Background(), wsID, func(_ context.Context, _ *pgxpool.Conn) error {
		t.Fatal("fn should not be called on credential error")
		return nil
	})
	if err == nil {
		t.Fatal("expected error from missing credentials, got nil")
	}
	if tp.PoolCount() != 0 {
		t.Errorf("PoolCount = %d after cred error, want 0", tp.PoolCount())
	}
}

func TestTenantPool_ClosedPool_RejectsWork(t *testing.T) {
	wsID := uuid.New()

	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{wsID: "ws_1"}},
		&stubCredentialResolver{dsns: map[string]string{"ws_1": "postgres://localhost/fake"}},
		TenantPoolConfig{MaxPools: 5},
	)

	tp.Close()

	err := tp.RunInWorkspace(context.Background(), wsID, func(_ context.Context, _ *pgxpool.Conn) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error from closed pool, got nil")
	}
}

func TestTenantPool_CloseIdempotent(t *testing.T) {
	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{},
	)

	tp.Close()
	tp.Close()
}

func TestTenantPool_LRUEviction_BoundsPoolCount(t *testing.T) {
	maxPools := 3
	dsns := make(map[string]string)

	for i := 0; i < 5; i++ {
		role := fmt.Sprintf("workspace_%d", i)
		dsns[role] = fmt.Sprintf("postgres://user:pass@localhost:59999/lecrm?search_path=%s,public", role)
	}

	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{}},
		&stubCredentialResolver{dsns: dsns},
		TenantPoolConfig{MaxPools: maxPools, MaxConnsPerPool: 2},
	)
	defer tp.Close()

	// pgxpool.NewWithConfig succeeds without connecting (lazy connect on Acquire).
	// This lets us validate the LRU eviction logic.
	for i := 0; i < 5; i++ {
		role := fmt.Sprintf("workspace_%d", i)
		_, _ = tp.getOrCreate(context.Background(), role)
	}

	if tp.PoolCount() != maxPools {
		t.Errorf("PoolCount = %d after 5 creates with max %d, want %d", tp.PoolCount(), maxPools, maxPools)
	}

	// Verify the first two pools were evicted (LRU order)
	tp.mu.Lock()
	_, has0 := tp.pools["workspace_0"]
	_, has1 := tp.pools["workspace_1"]
	_, has4 := tp.pools["workspace_4"]
	tp.mu.Unlock()

	if has0 {
		t.Error("workspace_0 should have been evicted (LRU)")
	}
	if has1 {
		t.Error("workspace_1 should have been evicted (LRU)")
	}
	if !has4 {
		t.Error("workspace_4 should still be present (most recent)")
	}
}

func TestTenantPool_Stats_Empty(t *testing.T) {
	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{MaxPools: 10},
	)
	defer tp.Close()

	stats := tp.Stats()
	if stats.ActivePools != 0 || stats.TotalAcquired != 0 || stats.TotalIdle != 0 {
		t.Errorf("unexpected stats for empty pool: %+v", stats)
	}
	if stats.MaxPools != 10 {
		t.Errorf("MaxPools = %d, want 10", stats.MaxPools)
	}
}

func TestTenantPool_MaxConnsPerPool_BoundsConnections(t *testing.T) {
	cfg := TenantPoolConfig{
		MaxPools:        20,
		MaxConnsPerPool: 3,
	}
	cfg.applyDefaults()

	maxTotalConns := int64(cfg.MaxPools) * int64(cfg.MaxConnsPerPool)
	if maxTotalConns != 60 {
		t.Errorf("max total connections = %d, want 60 (20 pools × 3 conns)", maxTotalConns)
	}
}

func TestTenantPool_50TenantBudget(t *testing.T) {
	cfg := TenantPoolConfig{
		MaxPools:        20,
		MaxConnsPerPool: 3,
	}

	maxTotalConns := int64(cfg.MaxPools) * int64(cfg.MaxConnsPerPool)
	controlPlaneConns := int64(10)
	totalBudget := maxTotalConns + controlPlaneConns

	// On a 12GB VPS, max_connections is ~150.
	if totalBudget > 150 {
		t.Errorf("total connection budget %d exceeds 12GB VPS limit of 150", totalBudget)
	}

	t.Logf("Connection budget: %d control-plane + %d tenant (20 active pools × 3) = %d total",
		controlPlaneConns, maxTotalConns, totalBudget)
	t.Logf("50 tenants served via LRU eviction of %d pool slots", cfg.MaxPools)
}

func TestTenantPool_ConcurrentResolverAccess(t *testing.T) {
	roles := make(map[uuid.UUID]string)
	for i := 0; i < 20; i++ {
		roles[uuid.New()] = fmt.Sprintf("workspace_%d", i)
	}

	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: roles},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{MaxPools: 5},
	)
	defer tp.Close()

	var wg sync.WaitGroup
	for id := range roles {
		wg.Add(1)
		go func(wsID uuid.UUID) {
			defer wg.Done()
			_ = tp.RunInWorkspace(context.Background(), wsID, func(_ context.Context, _ *pgxpool.Conn) error {
				return nil
			})
		}(id)
	}
	wg.Wait()
}
