// Package riversetup creates River's job tables inside a workspace's
// per-tenant river_<hex> schema and grants the application role (lecrm_api)
// the DML it needs to operate the queue from the main API pool.
//
// WHY THIS EXISTS
// ---------------
// core.lecrm_provision_workspace creates the river_<hex> SCHEMA
// (0001_init.sql step 5) but NOT River's tables — River ships its own
// migrations (rivermigrate) that must be run against each schema before a
// river.Client can Start there. The per-workspace river client in the API
// (apps/api/internal/sequences) targets river.Config.Schema = river_<hex>,
// so every provisioned workspace needs its river tables created once.
//
// OWNERSHIP / GRANTS
// ------------------
// river_<hex> is owned by the workspace role (CREATE SCHEMA ... AUTHORIZATION
// workspace_<hex>). lecrm_provisioner is a MEMBER of that role, so this package
// SET ROLEs to the workspace role (the schema owner) for the whole connection:
// rivermigrate then creates the river tables as the owner, and the subsequent
// GRANT/ALTER DEFAULT PRIVILEGES run as the owner too. lecrm_api (the API pool)
// gets USAGE on the schema + DML on the river tables so its river client can
// fetch/insert/work jobs. This mirrors core.lecrm_grant_app_role (0017) but for
// the river schema, and is run here (Go, provisioner-credentialed) rather than
// in the fully-restated provision function — avoiding that copy-paste footgun.
package riversetup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// RiverSchema returns the per-workspace river schema name (river_<32hex>),
// matching core.lecrm_provision_workspace and apps/api sequences.RiverSchema.
func RiverSchema(workspaceID uuid.UUID) string {
	return "river_" + strings.ReplaceAll(workspaceID.String(), "-", "")
}

// SetupWorkspace runs River's migrations in workspaceID's river_<hex> schema
// and grants lecrm_api the privileges to operate it. provisionerDSN must be a
// DSN for lecrm_provisioner (a member of roleName). roleName is the workspace
// role that owns the river schema (core.workspaces.role_name, i.e.
// workspace_<hex>). It is idempotent: rivermigrate skips already-applied
// versions and GRANT/ALTER DEFAULT PRIVILEGES are no-ops when already held.
func SetupWorkspace(ctx context.Context, provisionerDSN string, workspaceID uuid.UUID, roleName string, logger *slog.Logger) error {
	schema := RiverSchema(workspaceID)

	cfg, err := pgxpool.ParseConfig(provisionerDSN)
	if err != nil {
		return fmt.Errorf("riversetup: parse dsn: %w", err)
	}
	// Operate every connection AS the workspace role (the river schema owner) so
	// rivermigrate can CREATE in the schema and the grants below are issued by
	// the owner. SET ROLE needs no password — lecrm_provisioner is a member.
	roleSQL := "SET ROLE " + pgx.Identifier{roleName}.Sanitize()
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, roleSQL); err != nil {
			return fmt.Errorf("riversetup: set role %s: %w", roleName, err)
		}
		return nil
	}
	cfg.MaxConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("riversetup: open pool: %w", err)
	}
	defer pool.Close()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{
		Schema: schema,
		Logger: logger,
	})
	if err != nil {
		return fmt.Errorf("riversetup: new migrator for %s: %w", schema, err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("riversetup: migrate %s: %w", schema, err)
	}
	logger.InfoContext(ctx, "river migrations applied",
		"schema", schema, "versions_applied", len(res.Versions))

	// Grant lecrm_api the queue DML. Issued as the workspace role (owner of the
	// schema + the river tables rivermigrate just created). ALTER DEFAULT
	// PRIVILEGES (no FOR ROLE → for objects created by the current role) covers
	// any future river tables created by this same owner.
	q := pgx.Identifier{schema}.Sanitize()
	grants := []string{
		"GRANT USAGE ON SCHEMA " + q + " TO lecrm_api",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + q + " TO lecrm_api",
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA " + q + " TO lecrm_api",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA " + q + " GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lecrm_api",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA " + q + " GRANT USAGE, SELECT ON SEQUENCES TO lecrm_api",
	}
	for _, g := range grants {
		if _, err := pool.Exec(ctx, g); err != nil {
			return fmt.Errorf("riversetup: grant (%s): %w", g, err)
		}
	}
	logger.InfoContext(ctx, "river schema granted to lecrm_api", "schema", schema)
	return nil
}
