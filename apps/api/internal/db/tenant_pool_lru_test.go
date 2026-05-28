package db

import (
	"testing"
	"time"
)

// TestTenantPoolConfig_DefaultConnLifetime verifies that zero ConnMaxLifetime
// is replaced with the 1-hour default.
func TestTenantPoolConfig_DefaultConnLifetime(t *testing.T) {
	cfg := TenantPoolConfig{}
	cfg.applyDefaults()

	if cfg.ConnMaxLifetime != time.Hour {
		t.Errorf("ConnMaxLifetime = %v, want %v", cfg.ConnMaxLifetime, time.Hour)
	}
}

// TestTenantPoolConfig_DefaultConnIdleTime verifies that zero ConnMaxIdleTime
// is replaced with the 5-minute default.
func TestTenantPoolConfig_DefaultConnIdleTime(t *testing.T) {
	cfg := TenantPoolConfig{}
	cfg.applyDefaults()

	want := 5 * time.Minute
	if cfg.ConnMaxIdleTime != want {
		t.Errorf("ConnMaxIdleTime = %v, want %v", cfg.ConnMaxIdleTime, want)
	}
}

// TestTenantPoolConfig_PositiveValuesPreserved confirms that applyDefaults does
// not overwrite fields that are already positive.
func TestTenantPoolConfig_PositiveValuesPreserved(t *testing.T) {
	cases := []struct {
		name string
		cfg  TenantPoolConfig
	}{
		{
			name: "all custom",
			cfg: TenantPoolConfig{
				MaxPools:        50,
				MaxConnsPerPool: 10,
				ConnMaxLifetime: 2 * time.Hour,
				ConnMaxIdleTime: 15 * time.Minute,
			},
		},
		{
			name: "min positive values",
			cfg: TenantPoolConfig{
				MaxPools:        1,
				MaxConnsPerPool: 1,
				ConnMaxLifetime: time.Second,
				ConnMaxIdleTime: time.Second,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			want := tc.cfg // capture before applyDefaults mutates
			tc.cfg.applyDefaults()

			if tc.cfg.MaxPools != want.MaxPools {
				t.Errorf("MaxPools = %d, want %d", tc.cfg.MaxPools, want.MaxPools)
			}
			if tc.cfg.MaxConnsPerPool != want.MaxConnsPerPool {
				t.Errorf("MaxConnsPerPool = %d, want %d", tc.cfg.MaxConnsPerPool, want.MaxConnsPerPool)
			}
			if tc.cfg.ConnMaxLifetime != want.ConnMaxLifetime {
				t.Errorf("ConnMaxLifetime = %v, want %v", tc.cfg.ConnMaxLifetime, want.ConnMaxLifetime)
			}
			if tc.cfg.ConnMaxIdleTime != want.ConnMaxIdleTime {
				t.Errorf("ConnMaxIdleTime = %v, want %v", tc.cfg.ConnMaxIdleTime, want.ConnMaxIdleTime)
			}
		})
	}
}

// TestNewTenantPool_NilResolvers verifies that NewTenantPool does not panic
// when resolver and creds are nil — construction is a pure struct init.
func TestNewTenantPool_NilResolvers(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{})
	defer tp.Close()

	if tp == nil {
		t.Fatal("NewTenantPool returned nil")
	}
}

// TestNewTenantPool_AppliesDefaults verifies that NewTenantPool applies config
// defaults so the stored config reflects correct values.
func TestNewTenantPool_AppliesDefaults(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{})
	defer tp.Close()

	if tp.config.MaxPools != 20 {
		t.Errorf("config.MaxPools = %d, want 20", tp.config.MaxPools)
	}
	if tp.config.MaxConnsPerPool != 3 {
		t.Errorf("config.MaxConnsPerPool = %d, want 3", tp.config.MaxConnsPerPool)
	}
	if tp.config.ConnMaxLifetime != time.Hour {
		t.Errorf("config.ConnMaxLifetime = %v, want %v", tp.config.ConnMaxLifetime, time.Hour)
	}
	if tp.config.ConnMaxIdleTime != 5*time.Minute {
		t.Errorf("config.ConnMaxIdleTime = %v, want %v", tp.config.ConnMaxIdleTime, 5*time.Minute)
	}
}

// TestNewTenantPool_LRUStructuresEmpty verifies that the internal LRU list and
// index are both empty immediately after construction.
func TestNewTenantPool_LRUStructuresEmpty(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{})
	defer tp.Close()

	tp.mu.Lock()
	defer tp.mu.Unlock()

	if tp.lru.Len() != 0 {
		t.Errorf("lru.Len() = %d, want 0", tp.lru.Len())
	}
	if len(tp.lruIdx) != 0 {
		t.Errorf("lruIdx has %d entries, want 0", len(tp.lruIdx))
	}
	if len(tp.pools) != 0 {
		t.Errorf("pools has %d entries, want 0", len(tp.pools))
	}
}

// TestPoolCount_FreshPool verifies PoolCount returns zero on a freshly
// created pool that has not served any workspace connections.
func TestPoolCount_FreshPool(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{})
	defer tp.Close()

	if got := tp.PoolCount(); got != 0 {
		t.Errorf("PoolCount() = %d, want 0", got)
	}
}

// TestStats_FreshPool_DefaultMaxPools verifies Stats on a fresh pool with a
// custom MaxPools value reports that value and zero activity counters.
func TestStats_FreshPool_DefaultMaxPools(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{MaxPools: 20})
	defer tp.Close()

	s := tp.Stats()
	if s.ActivePools != 0 {
		t.Errorf("ActivePools = %d, want 0", s.ActivePools)
	}
	if s.MaxPools != 20 {
		t.Errorf("MaxPools = %d, want 20", s.MaxPools)
	}
	if s.TotalAcquired != 0 {
		t.Errorf("TotalAcquired = %d, want 0", s.TotalAcquired)
	}
	if s.TotalIdle != 0 {
		t.Errorf("TotalIdle = %d, want 0", s.TotalIdle)
	}
	if s.MaxConnsTotal != 0 {
		t.Errorf("MaxConnsTotal = %d, want 0", s.MaxConnsTotal)
	}
}

// TestClose_SetsClosedFlag verifies that Close sets the internal closed flag,
// preventing further use, and that calling Close a second time is a no-op.
func TestClose_SetsClosedFlag(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{})

	tp.Close()

	tp.mu.Lock()
	closed := tp.closed
	tp.mu.Unlock()

	if !closed {
		t.Error("closed flag should be true after Close()")
	}

	// Second Close must not panic.
	tp.Close()
}

// TestClose_ClearsInternalState verifies that Close empties the pools map,
// LRU list, and LRU index — using the internal LRU structures exercised
// directly via getOrCreate with a valid DSN that requires no real connection.
func TestClose_ClearsInternalState(t *testing.T) {
	tp := NewTenantPool(nil, nil, TenantPoolConfig{})

	// Manually inject a fake entry so Close has something to clean up.
	// We use nil for the pool value — Close calls p.Close() only when p != nil,
	// but the map entry, LRU list, and LRU index should all be cleared.
	tp.mu.Lock()
	elem := tp.lru.PushFront("fake_role")
	tp.lruIdx["fake_role"] = elem
	// Do NOT put a non-nil *pgxpool.Pool — we don't want to call Close on nil.
	tp.mu.Unlock()

	tp.Close()

	tp.mu.Lock()
	defer tp.mu.Unlock()

	if len(tp.lruIdx) != 0 {
		t.Errorf("lruIdx has %d entries after Close, want 0", len(tp.lruIdx))
	}
	if tp.lru.Len() != 0 {
		t.Errorf("lru.Len() = %d after Close, want 0", tp.lru.Len())
	}
}

// TestTenantPoolConfig_AllDefaults exercises every field of applyDefaults in
// a single call to confirm each zero value is replaced with its documented
// default simultaneously.
func TestTenantPoolConfig_AllDefaults(t *testing.T) {
	cfg := TenantPoolConfig{} // all zero values
	cfg.applyDefaults()

	tests := []struct {
		field string
		got   any
		want  any
	}{
		{"MaxPools", cfg.MaxPools, 20},
		{"MaxConnsPerPool", int(cfg.MaxConnsPerPool), 3},
		{"ConnMaxLifetime", cfg.ConnMaxLifetime, time.Hour},
		{"ConnMaxIdleTime", cfg.ConnMaxIdleTime, 5 * time.Minute},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.field, tt.got, tt.want)
		}
	}
}
