package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter_BurstThenRefill(t *testing.T) {
	l := New(2, 3) // 2 tokens/sec, burst 3
	base := time.Unix(1000, 0)
	l.now = func() time.Time { return base }

	// Burst of 3 succeeds, 4th is denied.
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("burst token %d denied", i)
		}
	}
	if l.Allow("k") {
		t.Fatal("4th call should be denied (bucket empty)")
	}

	// After 1s, 2 tokens refill.
	l.now = func() time.Time { return base.Add(time.Second) }
	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("expected 2 refilled tokens after 1s")
	}
	if l.Allow("k") {
		t.Fatal("3rd call after 1s should be denied")
	}
}

func TestLimiter_KeysIsolated(t *testing.T) {
	l := New(1, 1)
	base := time.Unix(0, 0)
	l.now = func() time.Time { return base }

	if !l.Allow("a") {
		t.Fatal("first call for a denied")
	}
	if !l.Allow("b") {
		t.Fatal("key b must have its own bucket")
	}
	if l.Allow("a") {
		t.Fatal("key a bucket should be empty")
	}
}

func TestLimiter_DisabledWhenRateZero(t *testing.T) {
	l := New(0, 0)
	for i := 0; i < 1000; i++ {
		if !l.Allow("k") {
			t.Fatal("zero rate must disable limiting")
		}
	}
}
