package jobs_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gbconsult/lecrm/apps/api/internal/jobs"
)

// ---------------------------------------------------------------------------
// Stubs for WorkspaceResolver / CredentialResolver (existing)
// ---------------------------------------------------------------------------

type stubResolver struct {
	roleName string
	err      error
}

func (s *stubResolver) WorkspaceRoleName(_ context.Context, _ uuid.UUID) (string, error) {
	return s.roleName, s.err
}

type stubCredResolver struct {
	dsn string
	err error
}

func (s *stubCredResolver) DSNForRole(_ context.Context, _ string) (string, error) {
	return s.dsn, s.err
}

func neverCalledFn(_ context.Context, _ *pgx.Conn) (int, error) {
	panic("fn must not be called when resolver/creds fail")
}

// ---------------------------------------------------------------------------
// RunWorkspaceJob error propagation (existing tests, updated)
// ---------------------------------------------------------------------------

func TestRunWorkspaceJob_ResolverError_Propagated(t *testing.T) {
	resolveErr := errors.New("no such workspace")

	_, err := jobs.RunWorkspaceJob[int](
		context.Background(),
		&stubResolver{err: resolveErr},
		&stubCredResolver{dsn: "postgres://localhost/nodb"},
		uuid.New(),
		neverCalledFn,
	)
	if err == nil {
		t.Fatal("expected error from resolver, got nil")
	}
	if !errors.Is(err, resolveErr) {
		t.Errorf("error chain does not contain resolveErr; got: %v", err)
	}
}

func TestRunWorkspaceJob_CredentialError_Propagated(t *testing.T) {
	credErr := errors.New("no credentials for role")

	_, err := jobs.RunWorkspaceJob[int](
		context.Background(),
		&stubResolver{roleName: "workspace_abc123"},
		&stubCredResolver{err: credErr},
		uuid.New(),
		neverCalledFn,
	)
	if err == nil {
		t.Fatal("expected error from cred resolver, got nil")
	}
	if !errors.Is(err, credErr) {
		t.Errorf("error chain does not contain credErr; got: %v", err)
	}
}

func TestProbeWorkspaceConnectivity_ResolverError(t *testing.T) {
	resolveErr := errors.New("workspace lookup failed")

	err := jobs.ProbeWorkspaceConnectivity(
		context.Background(),
		&stubResolver{err: resolveErr},
		&stubCredResolver{},
		uuid.New(),
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("ProbeWorkspaceConnectivity did not propagate resolver error; got %v", err)
	}
}

// ---------------------------------------------------------------------------
// withSafeExec tests (via exported WithSafeExecForTest)
// ---------------------------------------------------------------------------

func TestWithSafeExec_Success(t *testing.T) {
	mock := newMockExecutor("workspace_abc, public")
	wsID := uuid.New()

	err := jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.LockAcquired() {
		t.Error("advisory lock was not acquired")
	}
	if !mock.LockReleased() {
		t.Error("advisory lock was not released")
	}
}

func TestWithSafeExec_PanicRecovery_LockReleased(t *testing.T) {
	mock := newMockExecutor("workspace_abc, public")
	wsID := uuid.New()

	err := jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
		panic("simulated job panic")
	})

	if err == nil {
		t.Fatal("expected error from panic, got nil")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("error should mention panic, got: %v", err)
	}
	if !strings.Contains(err.Error(), "simulated job panic") {
		t.Errorf("error should contain panic value, got: %v", err)
	}
	if !mock.LockAcquired() {
		t.Error("advisory lock was not acquired before panic")
	}
	if !mock.LockReleased() {
		t.Error("advisory lock was not released after panic")
	}
}

func TestWithSafeExec_FnError_LockReleased(t *testing.T) {
	mock := newMockExecutor("workspace_abc, public")
	wsID := uuid.New()
	jobErr := errors.New("job failed")

	err := jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
		return jobErr
	})

	if !errors.Is(err, jobErr) {
		t.Errorf("expected job error, got: %v", err)
	}
	if !mock.LockReleased() {
		t.Error("advisory lock was not released after job error")
	}
}

func TestWithSafeExec_LockError_FnNotCalled(t *testing.T) {
	mock := newMockExecutor("workspace_abc, public")
	mock.SetLockErr(errors.New("lock acquire failed"))
	wsID := uuid.New()
	fnCalled := false

	err := jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
		fnCalled = true
		return nil
	})

	if err == nil {
		t.Fatal("expected lock error, got nil")
	}
	if fnCalled {
		t.Error("fn should not be called when lock acquisition fails")
	}
	if mock.LockReleased() {
		t.Error("lock should not be released when it was never acquired")
	}
}

func TestWithSafeExec_SearchPathMismatch_FnNotCalled(t *testing.T) {
	mock := newMockExecutor("wrong_schema, public")
	wsID := uuid.New()
	fnCalled := false

	err := jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
		fnCalled = true
		return nil
	})

	if err == nil {
		t.Fatal("expected search_path error, got nil")
	}
	if !strings.Contains(err.Error(), "search_path") {
		t.Errorf("error should mention search_path, got: %v", err)
	}
	if fnCalled {
		t.Error("fn should not be called when search_path is wrong")
	}
	if !mock.LockAcquired() {
		t.Error("lock should have been acquired before search_path check")
	}
	if !mock.LockReleased() {
		t.Error("lock should be released even on search_path mismatch")
	}
}

// ---------------------------------------------------------------------------
// Concurrency tests (simulated advisory locks via in-process mutex)
// ---------------------------------------------------------------------------

func TestConcurrentSameWorkspace_Serialized(t *testing.T) {
	mock := newConcurrentMockExecutor("workspace_abc, public")
	wsID := uuid.New()

	var mu sync.Mutex
	var timeline []string

	var wg sync.WaitGroup
	wg.Add(2)

	for i := range 2 {
		go func(n int) {
			defer wg.Done()
			_ = jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
				mu.Lock()
				timeline = append(timeline, "start")
				mu.Unlock()

				time.Sleep(20 * time.Millisecond)

				mu.Lock()
				timeline = append(timeline, "end")
				mu.Unlock()
				return nil
			})
		}(i)
	}

	wg.Wait()

	// Serialized: [start, end, start, end] — no interleaving
	if len(timeline) != 4 {
		t.Fatalf("expected 4 events, got %d: %v", len(timeline), timeline)
	}
	if timeline[0] != "start" || timeline[1] != "end" || timeline[2] != "start" || timeline[3] != "end" {
		t.Errorf("expected [start, end, start, end], got %v", timeline)
	}
}

func TestConcurrentDifferentWorkspaces_Parallel(t *testing.T) {
	mock := newConcurrentMockExecutor("workspace_abc, public")
	wsA := uuid.New()
	wsB := uuid.New()

	var running atomic.Int32
	var maxRunning atomic.Int32

	var wg sync.WaitGroup
	wg.Add(2)

	runJob := func(wsID uuid.UUID) {
		defer wg.Done()
		_ = jobs.WithSafeExecForTest(context.Background(), mock, wsID, "workspace_abc", func() error {
			cur := running.Add(1)
			for {
				old := maxRunning.Load()
				if cur <= old || maxRunning.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			running.Add(-1)
			return nil
		})
	}

	go runJob(wsA)
	go runJob(wsB)
	wg.Wait()

	if maxRunning.Load() < 2 {
		t.Errorf("expected parallel execution (max concurrent >= 2), got %d", maxRunning.Load())
	}
}

// ---------------------------------------------------------------------------
// Interface compliance tests
// ---------------------------------------------------------------------------

func TestSimpleJob_ImplementsJob(t *testing.T) {
	wsID := uuid.New()
	j := jobs.NewSimpleJob("contact.sync", wsID)

	var _ jobs.Job = j

	if j.Kind() != "contact.sync" {
		t.Errorf("Kind() = %q, want %q", j.Kind(), "contact.sync")
	}
	if j.WorkspaceID() != wsID {
		t.Errorf("WorkspaceID() = %s, want %s", j.WorkspaceID(), wsID)
	}
}

func TestRiverAdapter_ImplementsJobRunner(t *testing.T) {
	adapter := jobs.NewRiverAdapter(nil, nil, nil, nil)
	var _ jobs.JobRunner = adapter
}

func TestRiverAdapter_Enqueue_NilWorkspaceID_Error(t *testing.T) {
	adapter := jobs.NewRiverAdapter(nil, nil, nil, nil)
	err := adapter.Enqueue(context.Background(), jobs.NewSimpleJob("test", uuid.UUID{}))
	if err == nil {
		t.Fatal("expected error for nil workspace ID")
	}
}

func TestRiverAdapter_Enqueue_ValidJob(t *testing.T) {
	adapter := jobs.NewRiverAdapter(nil, nil, nil, nil)
	err := adapter.Enqueue(context.Background(), jobs.NewSimpleJob("test", uuid.New()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRiverAdapter_StartStop(t *testing.T) {
	adapter := jobs.NewRiverAdapter(nil, nil, nil, nil)
	ctx := context.Background()

	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := adapter.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock executors
// ---------------------------------------------------------------------------

// mockExecutor tracks advisory lock acquire/release calls and returns
// configurable search_path values. Safe for single-goroutine use.
type mockExecutor struct {
	mu            sync.Mutex
	searchPath    string
	lockAcquired  bool
	lockReleased  bool
	lockErr       error
	searchPathErr error
}

func newMockExecutor(searchPath string) *mockExecutor {
	return &mockExecutor{searchPath: searchPath}
}

func (m *mockExecutor) SetLockErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockErr = err
}

func (m *mockExecutor) LockAcquired() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockAcquired
}

func (m *mockExecutor) LockReleased() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockReleased
}

func (m *mockExecutor) Exec(_ context.Context, sql string, _ ...any) (jobs.CommandTag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.Contains(sql, "pg_advisory_lock(") && !strings.Contains(sql, "unlock") {
		m.lockAcquired = true
		return jobs.CommandTag{}, m.lockErr
	}
	if strings.Contains(sql, "pg_advisory_unlock(") {
		m.lockReleased = true
		return jobs.CommandTag{}, nil
	}
	return jobs.CommandTag{}, nil
}

func (m *mockExecutor) QueryRow(_ context.Context, sql string, _ ...any) jobs.Row {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.Contains(sql, "search_path") {
		return &mockRow{val: m.searchPath, err: m.searchPathErr}
	}
	return &mockRow{}
}

// concurrentMockExecutor uses per-workspace mutexes to simulate
// Postgres advisory lock blocking behavior.
type concurrentMockExecutor struct {
	mu         sync.Mutex
	locks      map[uuid.UUID]*sync.Mutex
	searchPath string
}

func newConcurrentMockExecutor(searchPath string) *concurrentMockExecutor {
	return &concurrentMockExecutor{
		locks:      make(map[uuid.UUID]*sync.Mutex),
		searchPath: searchPath,
	}
}

func (m *concurrentMockExecutor) getOrCreateLock(wsID uuid.UUID) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.locks[wsID]; ok {
		return l
	}
	l := &sync.Mutex{}
	m.locks[wsID] = l
	return l
}

func (m *concurrentMockExecutor) Exec(_ context.Context, sql string, args ...any) (jobs.CommandTag, error) {
	if len(args) > 0 {
		if wsStr, ok := args[0].(string); ok {
			wsID, err := uuid.Parse(wsStr)
			if err == nil {
				if strings.Contains(sql, "pg_advisory_lock(") && !strings.Contains(sql, "unlock") {
					m.getOrCreateLock(wsID).Lock()
				}
				if strings.Contains(sql, "pg_advisory_unlock(") {
					m.getOrCreateLock(wsID).Unlock()
				}
			}
		}
	}
	return jobs.CommandTag{}, nil
}

func (m *concurrentMockExecutor) QueryRow(_ context.Context, sql string, _ ...any) jobs.Row {
	return &mockRow{val: m.searchPath}
}

// mockRow implements pgx.Row for test assertions.
type mockRow struct {
	val string
	err error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) > 0 {
		if p, ok := dest[0].(*string); ok {
			*p = r.val
		}
	}
	return nil
}
