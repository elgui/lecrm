package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthCheck verifies the control-plane pool is responsive. Returns nil
// if a SELECT 1 completes within 5 seconds.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var one int
	if err := pool.QueryRow(hctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("db health check: %w", err)
	}
	return nil
}

// TenantPoolHealthy returns true if the tenant pool has not exceeded its
// connection budget. This is a lightweight check intended for readiness
// probes — it does not open connections.
func TenantPoolHealthy(tp *TenantPool) error {
	if tp == nil {
		return fmt.Errorf("tenant pool is nil")
	}
	stats := tp.Stats()
	if stats.ActivePools > stats.MaxPools {
		return fmt.Errorf("tenant pool over budget: %d/%d pools", stats.ActivePools, stats.MaxPools)
	}
	return nil
}
