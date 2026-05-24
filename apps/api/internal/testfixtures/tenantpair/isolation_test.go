//go:build integration

// Cross-tenant isolation tests — non-negotiable category (a) per
// docs/test-strategy.md §4.1.
//
// These tests provision two workspaces (acme-a and acme-b) against a
// testcontainers Postgres and assert that workspace context never leaks
// across tenants. The /v1/_test/workspaces handler (commit f69d24a) is
// the Sprint 3 endpoint surface; Sprint 4+ CRUD endpoints add isolation
// tests in their own PRs using the assertion helpers in assertions.go.
//
// Run:
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -v \
//	    -run TestIsolation ./internal/testfixtures/tenantpair/
//
// Minimum floor: ≥5 tests must be green at Sprint 3 close (§5).
// Full floor: ≥15 tests green before first Design Partner migration (§5).
// Hard-stop: any failure reverts the branch — no fix-forward (§6).

package tenantpair_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/gbconsult/lecrm/apps/api/internal/testfixtures/tenantpair"
)


// testEndpoint is the Sprint 3 tenant-scoped handler under test.
const testEndpoint = "/v1/_test/workspaces"

// workspaceListResponse mirrors the JSON shape returned by TestListHandler.
type workspaceListResponse struct {
	Workspace struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"workspace"`
	Items []struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"items"`
}

func decodeListResponse(t *testing.T, body io.Reader) workspaceListResponse {
	t.Helper()
	var r workspaceListResponse
	if err := json.NewDecoder(body).Decode(&r); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return r
}

// ────────────────────────────────────────────────────────────────────
// Core isolation tests (Sprint 3 surface: /v1/_test/workspaces)
// ────────────────────────────────────────────────────────────────────

// Test 1 — Tenant A receives its own workspace context.
func TestIsolation_TenantA_ContextMatchesSlugA(t *testing.T) {
	pair := tenantpair.Provision(t)

	resp, err := pair.A.Client().Get(pair.URL() + testEndpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", testEndpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}

	body := decodeListResponse(t, resp.Body)
	if body.Workspace.Slug != pair.A.Slug {
		t.Errorf("workspace.slug: got %q want %q", body.Workspace.Slug, pair.A.Slug)
	}
	if body.Workspace.ID != pair.A.ID.String() {
		t.Errorf("workspace.id: got %s want %s", body.Workspace.ID, pair.A.ID)
	}
}

// Test 2 — Tenant B receives its own workspace context.
func TestIsolation_TenantB_ContextMatchesSlugB(t *testing.T) {
	pair := tenantpair.Provision(t)

	resp, err := pair.B.Client().Get(pair.URL() + testEndpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", testEndpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}

	body := decodeListResponse(t, resp.Body)
	if body.Workspace.Slug != pair.B.Slug {
		t.Errorf("workspace.slug: got %q want %q", body.Workspace.Slug, pair.B.Slug)
	}
	if body.Workspace.ID != pair.B.ID.String() {
		t.Errorf("workspace.id: got %s want %s", body.Workspace.ID, pair.B.ID)
	}
}

// Test 3 — Workspace IDs are different; each tenant has a unique identity.
func TestIsolation_TenantIDsAreDistinct(t *testing.T) {
	pair := tenantpair.Provision(t)

	if pair.A.ID == pair.B.ID {
		t.Errorf("workspace IDs are equal: %s (both tenants have the same ID — isolation is broken)", pair.A.ID)
	}
	if pair.A.Slug == pair.B.Slug {
		t.Errorf("workspace slugs are equal: %q", pair.A.Slug)
	}
}

// Test 4 — Tenant A's workspace ID does not appear as Tenant B's workspace context.
func TestIsolation_ATenantIDNotLeakedToB(t *testing.T) {
	pair := tenantpair.Provision(t)

	resp, err := pair.B.Client().Get(pair.URL() + testEndpoint)
	if err != nil {
		t.Fatalf("GET as B: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decodeListResponse(t, resp.Body)
	if body.Workspace.ID == pair.A.ID.String() {
		t.Errorf("ISOLATION LEAK: B's workspace context returned A's workspace ID %s", pair.A.ID)
	}
}

// Test 5 — Tenant B's workspace ID does not appear as Tenant A's workspace context.
func TestIsolation_BTenantIDNotLeakedToA(t *testing.T) {
	pair := tenantpair.Provision(t)

	resp, err := pair.A.Client().Get(pair.URL() + testEndpoint)
	if err != nil {
		t.Fatalf("GET as A: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decodeListResponse(t, resp.Body)
	if body.Workspace.ID == pair.B.ID.String() {
		t.Errorf("ISOLATION LEAK: A's workspace context returned B's workspace ID %s", pair.B.ID)
	}
}

// Test 6 — Unknown subdomain returns 404, not A's or B's workspace data.
// ADR-009 §5.2: unknown slugs get 404 (not 401) to avoid enumeration oracle.
func TestIsolation_UnknownSubdomain_Returns404(t *testing.T) {
	pair := tenantpair.Provision(t)

	resp, err := pair.ClientWithHost("unknown-tenant.lecrm.test").Get(pair.URL() + testEndpoint)
	if err != nil {
		t.Fatalf("GET with unknown subdomain: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown subdomain: got status %d; want 404", resp.StatusCode)
	}
}

// Test 7 — No subdomain (root domain) returns 400, not workspace data.
func TestIsolation_NoSubdomain_Returns400(t *testing.T) {
	pair := tenantpair.Provision(t)

	resp, err := pair.ClientWithHost("lecrm.test").Get(pair.URL() + testEndpoint)
	if err != nil {
		t.Fatalf("GET with no subdomain: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("no subdomain: got status %d; want 400", resp.StatusCode)
	}
}

// Test 8 — Sequential interleaved requests: A → B → A each return correct context.
func TestIsolation_SequentialInterleavedRequests(t *testing.T) {
	pair := tenantpair.Provision(t)

	sequence := []struct {
		tenant   *tenantpair.Tenant
		wantSlug string
	}{
		{pair.A, pair.A.Slug},
		{pair.B, pair.B.Slug},
		{pair.A, pair.A.Slug},
		{pair.B, pair.B.Slug},
	}

	for i, step := range sequence {
		resp, err := step.tenant.Client().Get(pair.URL() + testEndpoint)
		if err != nil {
			t.Fatalf("step %d GET as %s: %v", i, step.tenant.Slug, err)
		}
		body := decodeListResponse(t, resp.Body)
		_ = resp.Body.Close()

		if body.Workspace.Slug != step.wantSlug {
			t.Errorf("step %d: workspace.slug = %q; want %q (request as %s leaked to wrong context)",
				i, body.Workspace.Slug, step.wantSlug, step.tenant.Slug)
		}
	}
}

// Test 9 — Parallel concurrent requests: A and B in parallel both get correct contexts.
// Validates that the workspace middleware has no shared mutable state that
// could cause context bleed under concurrent access.
func TestIsolation_ParallelConcurrentRequests(t *testing.T) {
	pair := tenantpair.Provision(t)

	const workers = 10
	var wg sync.WaitGroup
	errs := make(chan string, workers*2)

	check := func(tenant *tenantpair.Tenant) {
		defer wg.Done()
		resp, err := tenant.Client().Get(pair.URL() + testEndpoint)
		if err != nil {
			errs <- fmt.Sprintf("GET as %s: %v", tenant.Slug, err)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body := decodeListResponse(t, resp.Body)
		if body.Workspace.Slug != tenant.Slug {
			errs <- fmt.Sprintf("ISOLATION LEAK: concurrent request as %s got slug %q",
				tenant.Slug, body.Workspace.Slug)
		}
	}

	wg.Add(workers * 2)
	for range workers {
		go check(pair.A)
		go check(pair.B)
	}
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

// Test 10 — AssertNoCrossRead helper: B cannot read A's workspace context as its own.
func TestIsolation_AssertNoCrossRead_BCannotReadA(t *testing.T) {
	pair := tenantpair.Provision(t)
	tenantpair.AssertNoCrossRead(t, pair, pair.A, pair.B, testEndpoint, nil)
}

// Test 11 — AssertNoCrossList helper: A's slug is not B's workspace context.
func TestIsolation_AssertNoCrossList_ASlugNotInBContext(t *testing.T) {
	pair := tenantpair.Provision(t)
	tenantpair.AssertNoCrossList(t, pair, pair.A, pair.B, testEndpoint)
}


