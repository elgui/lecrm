package auth

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RevocationChecker tests whether a session has been revoked.
// Implementations must be safe for concurrent use.
type RevocationChecker interface {
	IsRevoked(ctx context.Context, jti uuid.UUID, userID uuid.UUID, issuedAt int64) (bool, error)
}

// RevokeSession records a single session revocation.
func RevokeSession(ctx context.Context, pool *pgxpool.Pool, jti uuid.UUID, userID uuid.UUID, expiresAt time.Time) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO core.session_revocations (jti, user_id, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (jti) DO NOTHING`,
		jti, userID, expiresAt)
	return err
}

// RevokeAllUserSessions marks all sessions for a user as revoked.
func RevokeAllUserSessions(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO core.user_revocations (user_id, revoked_at) VALUES ($1, now())
		 ON CONFLICT (user_id) DO UPDATE SET revoked_at = now()`,
		userID)
	return err
}

// CleanExpiredRevocations removes revocation entries whose tokens have
// naturally expired. Safe to run from a cron/River job.
func CleanExpiredRevocations(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx,
		`DELETE FROM core.session_revocations WHERE expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RevocationCache is a production RevocationChecker backed by Postgres
// with an in-memory bloom filter for the hot path. The bloom filter is
// rebuilt every refreshInterval from the DB; false positives fall through
// to a point query.
type RevocationCache struct {
	pool            *pgxpool.Pool
	logger          *slog.Logger
	refreshInterval time.Duration

	// atomicFilter is an *bloomFilter swapped atomically on refresh.
	atomicFilter unsafe.Pointer

	mu           sync.RWMutex
	userRevoked  map[uuid.UUID]int64 // user_id → revoked_at unix

	stopOnce sync.Once
	stopCh   chan struct{}
}

func NewRevocationCache(pool *pgxpool.Pool, logger *slog.Logger, refreshInterval time.Duration) *RevocationCache {
	if refreshInterval <= 0 {
		refreshInterval = 30 * time.Second
	}
	rc := &RevocationCache{
		pool:            pool,
		logger:          logger,
		refreshInterval: refreshInterval,
		userRevoked:     make(map[uuid.UUID]int64),
		stopCh:          make(chan struct{}),
	}
	empty := newBloomFilter(100, 6)
	atomic.StorePointer(&rc.atomicFilter, unsafe.Pointer(empty))
	return rc
}

// Start begins the background refresh loop. Call Stop() to clean up.
func (rc *RevocationCache) Start(ctx context.Context) {
	rc.refresh(ctx)
	go rc.loop(ctx)
}

func (rc *RevocationCache) Stop() {
	rc.stopOnce.Do(func() { close(rc.stopCh) })
}

func (rc *RevocationCache) loop(ctx context.Context) {
	ticker := time.NewTicker(rc.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rc.refresh(ctx)
		case <-rc.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (rc *RevocationCache) refresh(ctx context.Context) {
	jtis, err := rc.loadRevokedJTIs(ctx)
	if err != nil {
		rc.logger.Error("revocation cache: failed to load JTIs", "err", err)
		return
	}

	n := len(jtis)
	if n < 100 {
		n = 100
	}
	bf := newBloomFilter(uint(n), 6)
	for _, jti := range jtis {
		bf.add(jti[:])
	}
	atomic.StorePointer(&rc.atomicFilter, unsafe.Pointer(bf))

	users, err := rc.loadUserRevocations(ctx)
	if err != nil {
		rc.logger.Error("revocation cache: failed to load user revocations", "err", err)
		return
	}
	rc.mu.Lock()
	rc.userRevoked = users
	rc.mu.Unlock()
}

func (rc *RevocationCache) loadRevokedJTIs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := rc.pool.Query(ctx,
		`SELECT jti FROM core.session_revocations WHERE expires_at > now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jtis []uuid.UUID
	for rows.Next() {
		var jti uuid.UUID
		if err := rows.Scan(&jti); err != nil {
			return nil, err
		}
		jtis = append(jtis, jti)
	}
	return jtis, rows.Err()
}

func (rc *RevocationCache) loadUserRevocations(ctx context.Context) (map[uuid.UUID]int64, error) {
	rows, err := rc.pool.Query(ctx,
		`SELECT user_id, extract(epoch FROM revoked_at)::bigint FROM core.user_revocations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[uuid.UUID]int64)
	for rows.Next() {
		var uid uuid.UUID
		var ts int64
		if err := rows.Scan(&uid, &ts); err != nil {
			return nil, err
		}
		m[uid] = ts
	}
	return m, rows.Err()
}

// IsRevoked checks the bloom filter (fast path) and user revocation map.
// Falls through to a DB point query only on bloom-filter positives.
func (rc *RevocationCache) IsRevoked(ctx context.Context, jti uuid.UUID, userID uuid.UUID, issuedAt int64) (bool, error) {
	rc.mu.RLock()
	if revokedAt, ok := rc.userRevoked[userID]; ok && issuedAt <= revokedAt {
		rc.mu.RUnlock()
		return true, nil
	}
	rc.mu.RUnlock()

	bf := (*bloomFilter)(atomic.LoadPointer(&rc.atomicFilter))
	if !bf.test(jti[:]) {
		return false, nil
	}

	var exists bool
	err := rc.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM core.session_revocations WHERE jti = $1 AND expires_at > now())`,
		jti).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// bloomFilter is a minimal bloom filter using the Kirsch-Mitzenmacher
// optimization: two hash functions (FNV-1a 64-bit split into two 32-bit
// halves) generate k probes as h1 + i*h2.
type bloomFilter struct {
	bits    []uint64
	numBits uint
	numHash uint
}

func newBloomFilter(expectedItems uint, numHash uint) *bloomFilter {
	if expectedItems == 0 {
		expectedItems = 1
	}
	if numHash == 0 {
		numHash = 6
	}
	nbits := expectedItems * 10
	nwords := (nbits + 63) / 64
	return &bloomFilter{
		bits:    make([]uint64, nwords),
		numBits: nwords * 64,
		numHash: numHash,
	}
}

func (bf *bloomFilter) add(data []byte) {
	h1, h2 := bf.hashes(data)
	for i := uint(0); i < bf.numHash; i++ {
		pos := (h1 + i*h2) % bf.numBits
		bf.bits[pos/64] |= 1 << (pos % 64)
	}
}

func (bf *bloomFilter) test(data []byte) bool {
	h1, h2 := bf.hashes(data)
	for i := uint(0); i < bf.numHash; i++ {
		pos := (h1 + i*h2) % bf.numBits
		if bf.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false
		}
	}
	return true
}

func (bf *bloomFilter) hashes(data []byte) (uint, uint) {
	h := fnv.New64a()
	h.Write(data)
	sum := h.Sum64()
	return uint(sum >> 32), uint(uint32(sum))
}

// NopRevocationChecker always returns false (no revocation). Used when
// the revocation subsystem is not yet wired (e.g., missing DB tables
// during migration windows).
type NopRevocationChecker struct{}

func (NopRevocationChecker) IsRevoked(context.Context, uuid.UUID, uuid.UUID, int64) (bool, error) {
	return false, nil
}

// MemoryRevocationChecker is a test-only in-memory checker.
type MemoryRevocationChecker struct {
	mu          sync.RWMutex
	revokedJTIs map[uuid.UUID]struct{}
	userRevoked map[uuid.UUID]int64
}

func NewMemoryRevocationChecker() *MemoryRevocationChecker {
	return &MemoryRevocationChecker{
		revokedJTIs: make(map[uuid.UUID]struct{}),
		userRevoked: make(map[uuid.UUID]int64),
	}
}

func (m *MemoryRevocationChecker) RevokeJTI(jti uuid.UUID) {
	m.mu.Lock()
	m.revokedJTIs[jti] = struct{}{}
	m.mu.Unlock()
}

func (m *MemoryRevocationChecker) RevokeUser(userID uuid.UUID) {
	m.mu.Lock()
	m.userRevoked[userID] = time.Now().Unix()
	m.mu.Unlock()
}

func (m *MemoryRevocationChecker) IsRevoked(_ context.Context, jti uuid.UUID, userID uuid.UUID, issuedAt int64) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.revokedJTIs[jti]; ok {
		return true, nil
	}
	if revokedAt, ok := m.userRevoked[userID]; ok && issuedAt <= revokedAt {
		return true, nil
	}
	return false, nil
}

