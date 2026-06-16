package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// AcquireTx's success path needs a live workspace-role connection (covered by
// the integration suite, which provisions a tenant). These unit tests pin the
// failure envelope that resolves before any connection is opened, mirroring
// the RunInWorkspace error-path tests: on any pre-connection failure AcquireTx
// must return (ctx, nil-tx, nil-release, err) and leak no pool.

func TestAcquireTx_ResolverError(t *testing.T) {
	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{MaxPools: 5},
	)
	defer tp.Close()

	_, tx, release, err := tp.AcquireTx(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from missing workspace, got nil")
	}
	if tx != nil {
		t.Error("tx must be nil on resolver error")
	}
	if release != nil {
		t.Error("release must be nil on resolver error")
	}
	if tp.PoolCount() != 0 {
		t.Errorf("PoolCount = %d after resolver error, want 0", tp.PoolCount())
	}
}

func TestAcquireTx_CredentialError(t *testing.T) {
	wsID := uuid.New()
	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{wsID: "workspace_test"}},
		&stubCredentialResolver{dsns: map[string]string{}},
		TenantPoolConfig{MaxPools: 5},
	)
	defer tp.Close()

	_, tx, release, err := tp.AcquireTx(context.Background(), wsID)
	if err == nil {
		t.Fatal("expected error from missing credentials, got nil")
	}
	if tx != nil || release != nil {
		t.Error("tx and release must be nil on credential error")
	}
	if tp.PoolCount() != 0 {
		t.Errorf("PoolCount = %d after cred error, want 0", tp.PoolCount())
	}
}

func TestAcquireTx_ClosedPoolRejects(t *testing.T) {
	wsID := uuid.New()
	tp := NewTenantPool(
		&stubWorkspaceResolver{roles: map[uuid.UUID]string{wsID: "workspace_test"}},
		&stubCredentialResolver{dsns: map[string]string{"workspace_test": "postgres://localhost/x"}},
		TenantPoolConfig{MaxPools: 5},
	)
	tp.Close()

	_, tx, release, err := tp.AcquireTx(context.Background(), wsID)
	if err == nil {
		t.Fatal("expected error from closed pool, got nil")
	}
	if tx != nil || release != nil {
		t.Error("tx and release must be nil when the pool is closed")
	}
}
