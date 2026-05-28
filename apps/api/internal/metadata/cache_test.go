package metadata

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestDefCache_GetOnEmpty verifies that a newly created cache returns
// nil, false for any key.
func TestDefCache_GetOnEmpty(t *testing.T) {
	c := newDefCache()

	defs, ok := c.get("contact")
	if ok {
		t.Error("expected ok=false on empty cache, got true")
	}
	if defs != nil {
		t.Errorf("expected nil defs on empty cache, got %v", defs)
	}
}

// TestDefCache_PutThenGetWithinTTL verifies that a value stored with put
// is returned correctly by a subsequent get before the TTL expires.
func TestDefCache_PutThenGetWithinTTL(t *testing.T) {
	c := newDefCache()

	input := map[string]defEntry{
		"color": {propType: "string", required: false},
		"score": {propType: "number", required: true},
	}

	c.put("contact", input)

	defs, ok := c.get("contact")
	if !ok {
		t.Fatal("expected ok=true after put, got false")
	}
	if len(defs) != len(input) {
		t.Fatalf("expected %d entries, got %d", len(input), len(defs))
	}
	for key, want := range input {
		got, exists := defs[key]
		if !exists {
			t.Errorf("key %q missing from cached defs", key)
			continue
		}
		if got.propType != want.propType {
			t.Errorf("key %q: propType want %q, got %q", key, want.propType, got.propType)
		}
		if got.required != want.required {
			t.Errorf("key %q: required want %v, got %v", key, want.required, got.required)
		}
	}
}

// TestDefCache_GetAfterTTLExpiry verifies that entries whose fetchedAt
// time is older than cacheTTL are treated as expired. We manipulate
// fetchedAt directly because the test is in the same package.
func TestDefCache_GetAfterTTLExpiry(t *testing.T) {
	c := newDefCache()

	defs := map[string]defEntry{
		"status": {propType: "enum"},
	}
	c.put("contact", defs)

	// Wind back fetchedAt beyond the TTL.
	c.mu.Lock()
	entry := c.entries["contact"]
	entry.fetchedAt = time.Now().Add(-(cacheTTL + time.Second))
	c.entries["contact"] = entry
	c.mu.Unlock()

	got, ok := c.get("contact")
	if ok {
		t.Error("expected ok=false after TTL expiry, got true")
	}
	if got != nil {
		t.Errorf("expected nil defs after TTL expiry, got %v", got)
	}
}

// TestDefCache_InvalidateRemovesEntry verifies that invalidate deletes
// the entry so a subsequent get returns nil, false.
func TestDefCache_InvalidateRemovesEntry(t *testing.T) {
	c := newDefCache()

	c.put("deal", map[string]defEntry{"amount": {propType: "number"}})

	// Confirm it's present.
	if _, ok := c.get("deal"); !ok {
		t.Fatal("pre-condition: entry should be present before invalidate")
	}

	c.invalidate("deal")

	got, ok := c.get("deal")
	if ok {
		t.Error("expected ok=false after invalidate, got true")
	}
	if got != nil {
		t.Errorf("expected nil defs after invalidate, got %v", got)
	}
}

// TestDefCache_InvalidateUnknownKeyIsSafe verifies that calling invalidate
// on a key that was never stored does not panic.
func TestDefCache_InvalidateUnknownKeyIsSafe(t *testing.T) {
	c := newDefCache()
	// Must not panic.
	c.invalidate("does-not-exist")
}

// TestDefCache_EvictsOldestWhenFull fills the cache to cacheMaxSize entries
// and then puts one more. The entry with the oldest fetchedAt must be evicted.
func TestDefCache_EvictsOldestWhenFull(t *testing.T) {
	c := newDefCache()

	// Insert cacheMaxSize entries with ascending fetchedAt so we know
	// which one is oldest.
	base := time.Now().Add(-time.Duration(cacheMaxSize+1) * time.Second)

	for i := 0; i < cacheMaxSize; i++ {
		key := fmt.Sprintf("type-%d", i)
		c.mu.Lock()
		c.entries[key] = cacheEntry{
			defs:      map[string]defEntry{},
			fetchedAt: base.Add(time.Duration(i) * time.Second),
		}
		c.mu.Unlock()
	}

	if len(c.entries) != cacheMaxSize {
		t.Fatalf("pre-condition: expected %d entries, got %d", cacheMaxSize, len(c.entries))
	}

	// "type-0" has the oldest fetchedAt and should be evicted.
	oldestKey := "type-0"

	// Insert one more via the public put method (which triggers eviction).
	c.put("newcomer", map[string]defEntry{"field": {propType: "string"}})

	if len(c.entries) != cacheMaxSize {
		t.Errorf("after eviction+insert: expected %d entries, got %d", cacheMaxSize, len(c.entries))
	}

	if _, found := c.entries[oldestKey]; found {
		t.Errorf("oldest entry %q should have been evicted, but it's still present", oldestKey)
	}

	// The newly added entry must be present.
	if _, ok := c.get("newcomer"); !ok {
		t.Error("newly inserted entry 'newcomer' not found after eviction")
	}
}

// TestDefCache_ConcurrentGetPutNoPanic verifies that concurrent get and put
// calls do not panic or deadlock (data-race check requires -race flag).
func TestDefCache_ConcurrentGetPutNoPanic(t *testing.T) {
	c := newDefCache()

	var wg sync.WaitGroup
	const goroutines = 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("type-%d", n%5) // intentional key overlap
			defs := map[string]defEntry{
				"field": {propType: "string"},
			}
			c.put(key, defs)
			c.get(key)
			c.invalidate(key)
			c.get(key)
		}(i)
	}

	wg.Wait()
}
