package sequences

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// TestRiverSchema_MatchesProvisionFormula pins RiverSchema to the SQL in
// core.lecrm_provision_workspace (0001_init.sql step 5): the literal prefix
// "river_" followed by the workspace UUID, lowercased with every hyphen
// removed. The Go value MUST equal that or the river client targets the
// wrong schema.
func TestRiverSchema_MatchesProvisionFormula(t *testing.T) {
	id := uuid.MustParse("550E8400-E29B-41D4-A716-446655440000")

	// Independent recomputation of the SQL: strip hyphens, lowercase, prefix.
	want := "river_" + strings.ToLower(strings.ReplaceAll(id.String(), "-", ""))
	got := RiverSchema(id)
	if got != want {
		t.Fatalf("RiverSchema = %q, want %q", got, want)
	}

	// Concrete expectation, spelled out so an encoding change is obvious.
	const expected = "river_550e8400e29b41d4a716446655440000"
	if got != expected {
		t.Fatalf("RiverSchema = %q, want %q", got, expected)
	}
}

// TestRiverSchema_ShapeAndLength asserts the derived name is 38 chars
// (river_ + 32 hex), lowercase, and within river's schema-name limits
// (regex ^[a-zA-Z_][a-zA-Z0-9_]*$, max 46 chars for the default topics).
func TestRiverSchema_ShapeAndLength(t *testing.T) {
	s := RiverSchema(uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff"))
	if !strings.HasPrefix(s, "river_") {
		t.Errorf("schema %q missing river_ prefix", s)
	}
	if len(s) != len("river_")+32 {
		t.Errorf("schema %q length = %d, want %d", s, len(s), len("river_")+32)
	}
	if s != strings.ToLower(s) {
		t.Errorf("schema %q is not lowercase", s)
	}
	if strings.Contains(s, "-") {
		t.Errorf("schema %q still contains hyphens", s)
	}
}

// TestWorkspaceRiverConfig_TargetsTenantSchema verifies the config builder
// points the client at the workspace's per-tenant schema and wires a default
// queue with the requested parallelism.
func TestWorkspaceRiverConfig_TargetsTenantSchema(t *testing.T) {
	id := uuid.New()
	workers := river.NewWorkers()
	cfg := WorkspaceRiverConfig(id, workers, 4)

	if cfg.Schema != RiverSchema(id) {
		t.Errorf("config Schema = %q, want %q", cfg.Schema, RiverSchema(id))
	}
	if cfg.Workers != workers {
		t.Error("config did not carry the provided workers bundle")
	}
	q, ok := cfg.Queues[river.QueueDefault]
	if !ok {
		t.Fatalf("config has no default queue; queues = %v", cfg.Queues)
	}
	if q.MaxWorkers != 4 {
		t.Errorf("default queue MaxWorkers = %d, want 4", q.MaxWorkers)
	}
}

// TestWorkspaceRiverConfig_DefaultsQueueWorkers verifies a non-positive
// queueMaxWorkers falls back to the package default.
func TestWorkspaceRiverConfig_DefaultsQueueWorkers(t *testing.T) {
	cfg := WorkspaceRiverConfig(uuid.New(), river.NewWorkers(), 0)
	if cfg.Queues[river.QueueDefault].MaxWorkers != defaultQueueMaxWorkers {
		t.Errorf("MaxWorkers = %d, want default %d",
			cfg.Queues[river.QueueDefault].MaxWorkers, defaultQueueMaxWorkers)
	}
}
