// Package ratelimit implements a per-key token-bucket limiter used by
// the MCP adapter to bound request rate per (workspace_id, token_id)
// tuple. A heavy AI agent hammering tools/call for one workspace must
// not starve another tenant's reads.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter is a token-bucket limiter keyed by an arbitrary string. Each
// key gets its own bucket of `burst` tokens refilling at `rate` tokens
// per second. It is safe for concurrent use.
type Limiter struct {
	rate  float64 // tokens per second
	burst float64 // bucket capacity

	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time // injectable clock for tests
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a Limiter granting `burst` immediate tokens per key and
// refilling at `ratePerSec` tokens/second.
func New(ratePerSec, burst float64) *Limiter {
	return &Limiter{
		rate:    ratePerSec,
		burst:   burst,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// Allow reports whether one token is available for key, consuming it
// when so. A zero or negative rate disables limiting (always allows).
func (l *Limiter) Allow(key string) bool {
	if l.rate <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		// New key starts full, minus the token this call consumes.
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
