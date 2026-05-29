package mcpserver

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/capability"
)

func TestInitializeAdvertisesResources(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	resp := decodeResp(t, rec)
	res := resp.Result.(map[string]any)
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["resources"]; !ok {
		t.Fatal("server must advertise the resources capability")
	}
}

func TestResourcesListIncludesWorkspaceSchema(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":2,"method":"resources/list"}`)
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	resources := res["resources"].([]any)
	var found bool
	for _, r := range resources {
		if r.(map[string]any)["uri"].(string) == resourceWorkspaceSchema {
			found = true
		}
	}
	if !found {
		t.Fatalf("resources/list must include %q; got %v", resourceWorkspaceSchema, resources)
	}
}

func TestResourcesReadReturnsWorkspaceSchema(t *testing.T) {
	fr := &fakeReader{schema: capability.MCPWorkspaceSchema{
		Contact: []capability.MCPPropertyDef{
			{Key: "lead_score", Type: "number"},
			{Key: "cms", Type: "enum", AllowedValues: []string{"wordpress", "shopify"}},
		},
		Deal: []capability.MCPPropertyDef{
			{Key: "renewal", Type: "boolean", Required: true},
		},
	}}
	srv := newTestServer(fr)
	body := `{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"` + resourceWorkspaceSchema + `"}}`
	rec := rpc(t, srv, testWS, body)
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	// Scoped to the header workspace.
	if fr.lastWS != uuid.MustParse(testWS) {
		t.Fatalf("reader scoped to ws %s, want %s", fr.lastWS, testWS)
	}
	res := resp.Result.(map[string]any)
	contents := res["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("want 1 content item, got %d", len(contents))
	}
	item := contents[0].(map[string]any)
	if item["uri"].(string) != resourceWorkspaceSchema {
		t.Fatalf("content uri = %q", item["uri"])
	}
	if item["mimeType"].(string) != "application/json" {
		t.Fatalf("mimeType = %q", item["mimeType"])
	}
	text := item["text"].(string)
	// Carries the workspace's real fields and allowed values.
	for _, want := range []string{"lead_score", "cms", "wordpress", "renewal", testWS} {
		if !strings.Contains(text, want) {
			t.Fatalf("schema text missing %q: %s", want, text)
		}
	}
	// Compact serialisation (token-efficient): no indentation newlines.
	if strings.Contains(text, "\n") {
		t.Fatalf("schema must be compact (no newlines): %s", text)
	}
	// Unconstrained fields omit allowed_values / required (omitempty).
	if strings.Contains(text, `"allowed_values":[]`) || strings.Contains(text, `"required":false`) {
		t.Fatalf("compact schema must omit empty allowed_values/false required: %s", text)
	}
}

func TestResourcesReadEmptyWorkspaceSchema(t *testing.T) {
	srv := newTestServer(&fakeReader{}) // no definitions
	body := `{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"` + resourceWorkspaceSchema + `"}}`
	rec := rpc(t, srv, testWS, body)
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	text := res["contents"].([]any)[0].(map[string]any)["text"].(string)
	// A workspace with no custom properties returns a valid, complete shape:
	// empty arrays, never null.
	if !strings.Contains(text, `"contact":[]`) || !strings.Contains(text, `"deal":[]`) {
		t.Fatalf("empty schema must serialise empty arrays, got: %s", text)
	}
}

func TestResourcesReadDifferentWorkspacesScopedSeparately(t *testing.T) {
	const wsB = "22222222-2222-2222-2222-222222222222"
	srv := newTestServer(&fakeReader{})
	body := func(ws string) string {
		return `{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"` + resourceWorkspaceSchema + `"}}`
	}
	textFor := func(ws string) string {
		rec := rpc(t, srv, ws, body(ws))
		resp := decodeResp(t, rec)
		if resp.Error != nil {
			t.Fatalf("ws %s: unexpected error %+v", ws, resp.Error)
		}
		res := resp.Result.(map[string]any)
		return res["contents"].([]any)[0].(map[string]any)["text"].(string)
	}
	// Each caller's schema echoes its own workspace id — the dispatch never
	// crosses workspaces (per-workspace isolation; ADR-012 §5).
	if a := textFor(testWS); !strings.Contains(a, testWS) {
		t.Fatalf("ws A schema must carry its own id: %s", a)
	}
	if b := textFor(wsB); !strings.Contains(b, wsB) || strings.Contains(b, testWS) {
		t.Fatalf("ws B schema must carry only its own id: %s", b)
	}
}

func TestResourcesReadUnknownURIIsInvalidParams(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	body := `{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"lecrm://workspace/bogus"}}`
	rec := rpc(t, srv, testWS, body)
	resp := decodeResp(t, rec)
	if resp.Error == nil || resp.Error.Code != codeInvalidParams {
		t.Fatalf("unknown resource URI must be invalid-params, got %+v", resp.Error)
	}
}

func TestResourcesReadMissingWorkspaceHeaderRejected(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	body := `{"jsonrpc":"2.0","id":7,"method":"resources/read","params":{"uri":"` + resourceWorkspaceSchema + `"}}`
	rec := rpc(t, srv, "", body)
	resp := decodeResp(t, rec)
	if resp.Error == nil || resp.Error.Code != codeInvalidRequest {
		t.Fatalf("missing workspace header must be rejected, got %+v", resp.Error)
	}
}
