//go:build integration

// Integration test for ADR-009 §1.1 Test 1 — the /v1/_test/workspaces
// handler returning sqlc-typed rows from a live core.workspaces table.
//
// Requires the local Postgres compose stack from deploy/compose/postgres.yml
// to be up, with packages/db/migrations applied and at least one row in
// core.workspaces. Slug under test defaults to "acme" — override with
// LECRM_TEST_WORKSPACE_SLUG.
//
// Run:
//
//	set -a; source deploy/.env.dev; set +a
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -v \
//	    -run TestTestListHandler_Integration ./internal/workspace

package workspace_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	httpserver "github.com/gbconsult/lecrm/apps/api/internal/http"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

func TestTestListHandler_Integration(t *testing.T) {
	dsn := os.Getenv("LECRM_DATABASE_URL")
	if dsn == "" {
		t.Skip("LECRM_DATABASE_URL not set; skipping integration test")
	}
	slug := os.Getenv("LECRM_TEST_WORKSPACE_SLUG")
	if slug == "" {
		slug = "acme"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	deps := httpserver.RouterDeps{
		Logger:          logger,
		AuthHandler:     nil, // not exercised by this test
		Resolver:        &workspace.PoolResolver{Pool: pool},
		TestList:        &workspace.TestListHandler{Pool: pool, Logger: logger},
		CookieDomainTLD: "lecrm.test",
	}
	// AuthHandler.Register is called inside NewRouter; we can't pass nil.
	// Instead, hit the handler directly through its middleware composition
	// so we exercise the workspace context propagation without the auth tree.
	h := workspace.Middleware(logger, deps.Resolver, deps.CookieDomainTLD)(deps.TestList)

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = slug + ".lecrm.test"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body: %s", rr.Code, rr.Body.String())
	}

	var body struct {
		Workspace struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"workspace"`
		Items []struct {
			ID        string    `json:"id"`
			Slug      string    `json:"slug"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Workspace.Slug != slug {
		t.Errorf("workspace slug: got %q want %q", body.Workspace.Slug, slug)
	}
	if len(body.Items) == 0 {
		t.Errorf("items: got 0 rows; expected at least one workspace")
	}
}
