//go:build integration

package tenantpair

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// workspaceContextResponse is the shape returned by /v1/_test/workspaces and,
// by convention, any future tenant-scoped list endpoint that wraps its payload
// with the current workspace identity. CRUD endpoints in Sprint 4+ return
// the workspace context in the same envelope shape.
type workspaceContextResponse struct {
	Workspace struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"workspace"`
}

// decodeWorkspaceContext is a helper that reads and decodes the workspace
// context envelope from an HTTP response body.
func decodeWorkspaceContext(t *testing.T, body io.Reader) workspaceContextResponse {
	t.Helper()
	var resp workspaceContextResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode workspace context: %v", err)
	}
	return resp
}

// AssertNoCrossRead verifies that dst cannot observe src's workspace context
// when hitting endpoint. It checks that the workspace ID returned for dst's
// request is dst's own ID, NOT src's.
//
// For Sprint 3, this validates middleware-level context isolation using the
// /v1/_test/workspaces handler. Sprint 4+ passes CRUD endpoints where
// srcRecord is the resource ID written to src that must be absent from dst's
// response.
//
// The function signature matches the spec from docs/test-strategy.md §4.1:
// it takes an arbitrary srcRecord value for forward compatibility but uses
// the workspace context response for assertion at Sprint 3.
func AssertNoCrossRead(t *testing.T, pair *Pair, src, dst *Tenant, endpoint string, _ any) {
	t.Helper()

	resp, err := dst.Client().Get(pair.URL() + endpoint)
	if err != nil {
		t.Fatalf("AssertNoCrossRead: GET %s as %s: %v", endpoint, dst.Slug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("AssertNoCrossRead: %s returned %d: %s", endpoint, resp.StatusCode, body)
	}

	ctx := decodeWorkspaceContext(t, resp.Body)
	if ctx.Workspace.ID == src.ID.String() {
		t.Errorf("AssertNoCrossRead FAIL: dst %s received src %s workspace ID %s as its own context",
			dst.Slug, src.Slug, src.ID)
	}
	if ctx.Workspace.ID != dst.ID.String() {
		t.Errorf("AssertNoCrossRead FAIL: dst %s workspace context ID = %s; want %s",
			dst.Slug, ctx.Workspace.ID, dst.ID)
	}
}

// AssertNoCrossList verifies that hitting listEndpoint as dst returns no data
// owned by src. At Sprint 3, it checks that dst's workspace context is scoped
// to dst only. Sprint 4+ wires this to list endpoints where dst must see zero
// of src's records.
func AssertNoCrossList(t *testing.T, pair *Pair, src, dst *Tenant, listEndpoint string) {
	t.Helper()

	resp, err := dst.Client().Get(pair.URL() + listEndpoint)
	if err != nil {
		t.Fatalf("AssertNoCrossList: GET %s as %s: %v", listEndpoint, dst.Slug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("AssertNoCrossList: %s returned %d: %s", listEndpoint, resp.StatusCode, body)
	}

	ctx := decodeWorkspaceContext(t, resp.Body)
	if ctx.Workspace.Slug == src.Slug {
		t.Errorf("AssertNoCrossList FAIL: dst %s received src %s slug as its workspace context",
			dst.Slug, src.Slug)
	}
	if ctx.Workspace.Slug != dst.Slug {
		t.Errorf("AssertNoCrossList FAIL: dst %s workspace slug = %q; want %q",
			dst.Slug, ctx.Workspace.Slug, dst.Slug)
	}
}

// AssertNoCrossMutation verifies that dst cannot mutate src's resource.
// It sends a mutationMethod request to mutationEndpoint from dst's client
// and asserts the response is 404 (resource not found — NOT 403, which
// would leak resource existence per docs/test-strategy.md §4.1 and
// ADR-009 §5.2).
//
// At Sprint 3, no mutation endpoints exist; call this in Sprint 4+ when
// CRUD lands. It panics with a descriptive message if called before mutation
// endpoints are available to catch accidental early use.
func AssertNoCrossMutation(t *testing.T, pair *Pair, _, dst *Tenant, mutationMethod, mutationEndpoint, srcRecordID string) {
	t.Helper()

	if mutationEndpoint == "" || srcRecordID == "" {
		t.Fatal("AssertNoCrossMutation: mutationEndpoint and srcRecordID are required (Sprint 4+)")
	}

	url := fmt.Sprintf("%s%s/%s", pair.URL(), mutationEndpoint, srcRecordID)
	req, err := http.NewRequest(mutationMethod, url, nil)
	if err != nil {
		t.Fatalf("AssertNoCrossMutation: build request: %v", err)
	}
	req.Host = dst.Slug + "." + domainTLD

	resp, err := dst.Client().Do(req)
	if err != nil {
		t.Fatalf("AssertNoCrossMutation: %s %s as %s: %v", mutationMethod, url, dst.Slug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("AssertNoCrossMutation FAIL: expected 404 from %s %s as %s; got %d "+
			"(403 would leak resource existence; 200/204 is a cross-tenant mutation)",
			mutationMethod, url, dst.Slug, resp.StatusCode)
	}
}
