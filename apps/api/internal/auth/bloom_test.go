package auth

import (
	"fmt"
	"testing"
)

// TestBloomFilter_EmptyReturnsFalse verifies that a freshly created bloom
// filter has no bits set and returns false for arbitrary inputs.
func TestBloomFilter_EmptyReturnsFalse(t *testing.T) {
	bf := newBloomFilter(100, 6)

	// Try a variety of byte patterns; none should match.
	probes := [][]byte{
		{0x00},
		{0xFF},
		[]byte("hello"),
		[]byte("arbitrary data"),
		make([]byte, 16),
	}
	for _, p := range probes {
		if bf.test(p) {
			t.Errorf("empty bloom filter returned true for probe %q", p)
		}
	}
}

// TestBloomFilter_AddThenTest verifies that every item added is subsequently
// reported as present (no false negatives are possible in a bloom filter).
func TestBloomFilter_AddThenTest(t *testing.T) {
	bf := newBloomFilter(50, 6)

	items := [][]byte{
		[]byte("alpha"),
		[]byte("beta"),
		[]byte("gamma"),
		[]byte("delta"),
		[]byte("epsilon"),
	}

	for _, item := range items {
		bf.add(item)
	}

	for _, item := range items {
		if !bf.test(item) {
			t.Errorf("bloom filter returned false for added item %q", item)
		}
	}
}

// TestBloomFilter_FalsePositiveRate adds N items then checks M non-added items.
// With 10× bits per item and 6 hash functions the theoretical FP rate is ~1%.
// We assert it stays below 10% to give generous headroom.
func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	const (
		numAdded   = 1000
		numProbed  = 10000
		maxFPRate  = 0.10 // 10% ceiling
	)

	bf := newBloomFilter(numAdded, 6)

	// Add items using a prefix that won't collide with the probe prefix.
	for i := 0; i < numAdded; i++ {
		bf.add([]byte(fmt.Sprintf("added-item-%d", i)))
	}

	// Probe items that were never added.
	falsePositives := 0
	for i := 0; i < numProbed; i++ {
		if bf.test([]byte(fmt.Sprintf("probed-item-%d", i))) {
			falsePositives++
		}
	}

	fpRate := float64(falsePositives) / float64(numProbed)
	t.Logf("false-positive rate: %.4f (%d/%d)", fpRate, falsePositives, numProbed)
	if fpRate > maxFPRate {
		t.Errorf("false-positive rate %.4f exceeds ceiling %.4f", fpRate, maxFPRate)
	}
}

// TestBloomFilter_ZeroExpectedItems verifies that newBloomFilter does not
// panic or divide-by-zero when expectedItems is 0. The implementation
// substitutes 1 for 0, so the filter must behave correctly afterwards.
func TestBloomFilter_ZeroExpectedItems(t *testing.T) {
	// Must not panic.
	bf := newBloomFilter(0, 6)

	if bf == nil {
		t.Fatal("newBloomFilter(0, 6) returned nil")
	}
	if bf.numBits == 0 {
		t.Fatal("bloom filter with expectedItems=0 has zero bits")
	}

	item := []byte("test-item")

	// Before adding: must return false.
	if bf.test(item) {
		t.Error("empty bloom filter (zero expectedItems) returned true")
	}

	// After adding: must return true.
	bf.add(item)
	if !bf.test(item) {
		t.Error("bloom filter (zero expectedItems) returned false for added item")
	}
}

// TestBloomFilter_ZeroNumHash verifies that newBloomFilter substitutes a
// sensible default (6) when numHash is 0, so the filter remains functional.
func TestBloomFilter_ZeroNumHash(t *testing.T) {
	bf := newBloomFilter(100, 0)

	if bf == nil {
		t.Fatal("newBloomFilter(100, 0) returned nil")
	}
	if bf.numHash == 0 {
		t.Fatal("bloom filter has numHash == 0 after construction")
	}

	item := []byte("sentinel")
	bf.add(item)
	if !bf.test(item) {
		t.Error("bloom filter (zero numHash defaulted) returned false for added item")
	}
}
