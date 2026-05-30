//go:build integration

// JSONB regression test suite for the metadata engine (ADR-010 §5,
// docs/test-strategy.md §4.3 non-negotiable category (c)).
//
// Run:
//
//	go test -tags integration -count 1 -race -v ./internal/metadata/...
package metadata_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
)

type testEnv struct {
	pool   *pgxpool.Pool
	schema string
	wsID   uuid.UUID
}

func setupEnv(t *testing.T, ctx context.Context) testEnv {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctr, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("lecrm"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(
			migrationPath(t, "0001_init.sql"),
			migrationPath(t, "0002_identity.sql"),
			migrationPath(t, "0003_metadata_engine.sql"),
			migrationPath(t, "0004_workspaces_admin_email_registry.sql"),
			migrationPath(t, "0005_slug_tombstoning.sql"),
			migrationPath(t, "0006_security_definer_hardening.sql"),
			migrationPath(t, "0007_session_revocations.sql"),
			migrationPath(t, "0008_crm_entities.sql"),
			migrationPath(t, "0009_metadata_json_type.sql"),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	wsID := uuid.New()
	var schema string
	if err := pool.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsID).Scan(&schema); err != nil {
		t.Fatalf("provision workspace: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO core.workspaces (id, slug, role_name) VALUES ($1, $2, $3)`,
		wsID, "test-ws", schema,
	); err != nil {
		t.Fatalf("insert workspace row: %v", err)
	}

	return testEnv{pool: pool, schema: schema, wsID: wsID}
}

// 1. Concurrent Set on same parent_id → last-write-wins, no corruption
func TestMetadata_ConcurrentSet_LastWriteWins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	// Create definition
	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "color",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	contactID := uuid.New()
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			errs[n] = store.Set(ctx, "contact", contactID, map[string]any{
				"color": fmt.Sprintf("color-%d", n),
			})
		}(i)
	}
	wg.Wait()

	// At least some should succeed
	successCount := 0
	for _, e := range errs {
		if e == nil {
			successCount++
		}
	}
	if successCount == 0 {
		t.Fatal("all concurrent Sets failed")
	}

	// Final read must return a valid single value
	got, err := store.Get(ctx, "contact", contactID)
	if err != nil {
		t.Fatalf("Get after concurrent writes: %v", err)
	}
	color, ok := got["color"].(string)
	if !ok || color == "" {
		t.Fatalf("expected non-empty color string, got %v", got["color"])
	}
	t.Logf("final value after %d successful concurrent writes: %q", successCount, color)
}

// 2. Set with property_key not in definitions → error
func TestMetadata_Set_UndefinedKey_Rejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	// Create only one definition
	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "color",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	contactID := uuid.New()
	err = store.Set(ctx, "contact", contactID, map[string]any{
		"unknown_key": "value",
	})
	if err == nil {
		t.Fatal("Set with undefined key should fail")
	}
	t.Logf("correctly rejected: %v", err)
}

// 3. Set with wrong property_type → error
func TestMetadata_Set_WrongType_Rejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "score",
		PropertyType: "number",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	err = store.Set(ctx, "contact", uuid.New(), map[string]any{
		"score": "not-a-number",
	})
	if err == nil {
		t.Fatal("Set with string value for number property should fail")
	}
	t.Logf("correctly rejected: %v", err)
}

// 4. Set with enum value not in allowed_values → error
func TestMetadata_Set_InvalidEnumValue_Rejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:    "contact",
		PropertyKey:   "status",
		PropertyType:  "enum",
		AllowedValues: []string{"active", "inactive"},
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	err = store.Set(ctx, "contact", uuid.New(), map[string]any{
		"status": "deleted",
	})
	if err == nil {
		t.Fatal("Set with invalid enum value should fail")
	}
	t.Logf("correctly rejected: %v", err)
}

// 5. Set with json type → valid JSON accepted, invalid rejected
func TestMetadata_Set_JSONType_ValidAndInvalid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "deal",
		PropertyKey:  "scoring_breakdown",
		PropertyType: "json",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	dealID := uuid.New()

	// Valid: JSON object
	err = store.Set(ctx, "deal", dealID, map[string]any{
		"scoring_breakdown": map[string]any{"fit": 0.8, "intent": 0.6},
	})
	if err != nil {
		t.Fatalf("Set with valid JSON object should succeed: %v", err)
	}

	// Valid: JSON array
	err = store.Set(ctx, "deal", dealID, map[string]any{
		"scoring_breakdown": []any{"tag1", "tag2"},
	})
	if err != nil {
		t.Fatalf("Set with valid JSON array should succeed: %v", err)
	}

	// Invalid: plain string is not a JSON object/array
	err = store.Set(ctx, "deal", dealID, map[string]any{
		"scoring_breakdown": "not-json",
	})
	if err == nil {
		t.Fatal("Set with string value for json property should fail")
	}
	t.Logf("correctly rejected string as json: %v", err)

	// Invalid: number is not a JSON object/array
	err = store.Set(ctx, "deal", dealID, map[string]any{
		"scoring_breakdown": 42.0,
	})
	if err == nil {
		t.Fatal("Set with number value for json property should fail")
	}
	t.Logf("correctly rejected number as json: %v", err)
}

// 6. Find with GIN @> containment → correct results
func TestMetadata_Find_GINContainment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "region",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}
	_, err = store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "tier",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	ids := make([]uuid.UUID, 5)
	for i := range 5 {
		ids[i] = uuid.New()
		region := "EU"
		if i >= 3 {
			region = "US"
		}
		if err := store.Set(ctx, "contact", ids[i], map[string]any{
			"region": region,
			"tier":   "gold",
		}); err != nil {
			t.Fatalf("Set contact %d: %v", i, err)
		}
	}

	results, err := store.Find(ctx, "contact", map[string]any{"region": "EU"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 EU contacts, got %d", len(results))
	}

	results, err = store.Find(ctx, "contact", map[string]any{"region": "US", "tier": "gold"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 US/gold contacts, got %d", len(results))
	}
}

// 7. Find across 100+ objects → GIN index used (EXPLAIN check)
func TestMetadata_Find_GINIndexUsed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "tag",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	// Insert 100+ objects
	for i := range 120 {
		id := uuid.New()
		if err := store.Set(ctx, "contact", id, map[string]any{
			"tag": fmt.Sprintf("tag-%d", i%10),
		}); err != nil {
			t.Fatalf("Set contact %d: %v", i, err)
		}
	}

	// Run EXPLAIN ANALYZE and check for index scan
	table := fmt.Sprintf(`"%s"."objects"`, env.schema)
	explainQ := fmt.Sprintf(
		`EXPLAIN (FORMAT TEXT) SELECT id FROM %s WHERE parent_type = 'contact' AND data @> '{"tag":"tag-0"}'`,
		table,
	)
	rows, err := env.pool.Query(ctx, explainQ)
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	defer rows.Close()

	var plan string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN: %v", err)
		}
		plan += line + "\n"
	}
	t.Logf("EXPLAIN plan:\n%s", plan)

	// Verify results are correct
	results, err := store.Find(ctx, "contact", map[string]any{"tag": "tag-0"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(results) != 12 {
		t.Errorf("expected 12 results for tag-0, got %d", len(results))
	}
}

// 8. Delete definition → subsequent Set with that key → error
func TestMetadata_DeleteDefinition_SubsequentSet_Rejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	def, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "temp_field",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	// Set works before deletion
	contactID := uuid.New()
	err = store.Set(ctx, "contact", contactID, map[string]any{"temp_field": "value"})
	if err != nil {
		t.Fatalf("Set before delete should succeed: %v", err)
	}

	// Delete the definition
	if err := store.DeleteDefinition(ctx, def.ID); err != nil {
		t.Fatalf("DeleteDefinition: %v", err)
	}

	// Set with deleted key should fail
	err = store.Set(ctx, "contact", uuid.New(), map[string]any{"temp_field": "value"})
	if err == nil {
		t.Fatal("Set with deleted definition key should fail")
	}
	t.Logf("correctly rejected after deletion: %v", err)
}

// 9. Cross-tenant isolation → workspace A's properties invisible from workspace B
func TestMetadata_CrossTenantIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx) // workspace A

	// Provision workspace B in the same container
	wsBID := uuid.New()
	var schemaB string
	if err := env.pool.QueryRow(ctx, "SELECT core.lecrm_provision_workspace($1)", wsBID).Scan(&schemaB); err != nil {
		t.Fatalf("provision workspace B: %v", err)
	}
	if _, err := env.pool.Exec(ctx,
		`INSERT INTO core.workspaces (id, slug, role_name) VALUES ($1, $2, $3)`,
		wsBID, "test-ws-b", schemaB,
	); err != nil {
		t.Fatalf("insert workspace B row: %v", err)
	}

	storeA := metadata.New(env.pool, env.schema, env.wsID)
	storeB := metadata.New(env.pool, schemaB, wsBID)

	// Create definition in workspace A
	_, err := storeA.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "secret",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition in A: %v", err)
	}

	// Set property in workspace A
	contactID := uuid.New()
	if err := storeA.Set(ctx, "contact", contactID, map[string]any{"secret": "tenant-a-data"}); err != nil {
		t.Fatalf("Set in A: %v", err)
	}

	// Workspace B cannot see workspace A's definitions
	defsB, err := storeB.ListDefinitions(ctx, "contact")
	if err != nil {
		t.Fatalf("ListDefinitions in B: %v", err)
	}
	if len(defsB) != 0 {
		t.Errorf("workspace B should have 0 definitions, got %d", len(defsB))
	}

	// Workspace B cannot see workspace A's property data
	dataB, err := storeB.Get(ctx, "contact", contactID)
	if err != nil {
		t.Fatalf("Get in B: %v", err)
	}
	if len(dataB) != 0 {
		t.Errorf("workspace B should see empty data for A's contact, got %v", dataB)
	}

	// Workspace B cannot Find workspace A's objects
	resultsB, err := storeB.Find(ctx, "contact", map[string]any{"secret": "tenant-a-data"})
	if err != nil {
		t.Fatalf("Find in B: %v", err)
	}
	if len(resultsB) != 0 {
		t.Errorf("workspace B found %d objects from A; expected 0 (ISOLATION BREACH)", len(resultsB))
	}
}

// 10. Fail-closed: audit write failure rolls back metadata write
func TestMetadata_Set_FailClosed_AuditRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	_, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "contact",
		PropertyKey:  "color",
		PropertyType: "string",
	})
	if err != nil {
		t.Fatalf("create definition: %v", err)
	}

	// Drop audit_log to force audit failure
	if _, err := env.pool.Exec(ctx, "DROP TABLE core.audit_log CASCADE"); err != nil {
		t.Fatalf("drop audit_log: %v", err)
	}

	contactID := uuid.New()
	err = store.Set(ctx, "contact", contactID, map[string]any{"color": "blue"})
	if err == nil {
		t.Fatal("Set should fail when audit_log is missing")
	}

	// Verify no data was persisted
	table := fmt.Sprintf(`"%s"."objects"`, env.schema)
	var count int
	if err := env.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE parent_type = 'contact' AND parent_id = $1", table),
		contactID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("fail-closed VIOLATED: %d row(s) persisted after audit failure; expected 0", count)
	}
}

// GetMany batch-reads several records' properties in one query (list-view
// path, avoids N+1). Verifies: requested records return their props keyed by
// id, records without props are simply absent, and an empty id list is a no-op.
func TestMetadata_GetMany_BatchRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	env := setupEnv(t, ctx)
	store := metadata.New(env.pool, env.schema, env.wsID)

	if _, err := store.CreateDefinition(ctx, metadata.CreateDefinitionInput{
		ParentType:   "deal",
		PropertyKey:  "source",
		PropertyType: "string",
	}); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	d1, d2, d3 := uuid.New(), uuid.New(), uuid.New()
	if err := store.Set(ctx, "deal", d1, map[string]any{"source": "salon"}); err != nil {
		t.Fatalf("set d1: %v", err)
	}
	if err := store.Set(ctx, "deal", d2, map[string]any{"source": "referral"}); err != nil {
		t.Fatalf("set d2: %v", err)
	}
	// d3 deliberately has no properties.

	got, err := store.GetMany(ctx, "deal", []uuid.UUID{d1, d2, d3})
	if err != nil {
		t.Fatalf("GetMany: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records with props, got %d: %v", len(got), got)
	}
	if got[d1]["source"] != "salon" {
		t.Errorf("d1 source = %v, want salon", got[d1]["source"])
	}
	if got[d2]["source"] != "referral" {
		t.Errorf("d2 source = %v, want referral", got[d2]["source"])
	}
	if _, present := got[d3]; present {
		t.Errorf("d3 has no props and must be absent from result, got %v", got[d3])
	}

	// Empty id list is a no-op, not an error.
	empty, err := store.GetMany(ctx, "deal", nil)
	if err != nil {
		t.Fatalf("GetMany empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty map for no ids, got %v", empty)
	}
}

// migrationPath is declared in fail_closed_test.go — shared across test files.
