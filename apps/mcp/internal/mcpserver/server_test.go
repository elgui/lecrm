package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/capability"
	"github.com/gbconsult/lecrm/apps/mcp/internal/ratelimit"
	"github.com/gbconsult/lecrm/apps/mcp/internal/store"
)

// fakeReader records the workspace it was called with and returns canned
// data, so the JSON-RPC layer can be tested without a database.
type fakeReader struct {
	lastWS   uuid.UUID
	contact  capability.MCPContact
	notFound bool
	schema   capability.MCPWorkspaceSchema
}

func (f *fakeReader) ReadContact(_ context.Context, ws, id uuid.UUID) (capability.MCPContact, error) {
	f.lastWS = ws
	if f.notFound {
		return capability.MCPContact{}, store.ErrNotFound
	}
	f.contact.ID = id
	return f.contact, nil
}
func (f *fakeReader) ListContacts(_ context.Context, ws uuid.UUID, _ store.Page) (capability.MCPContacts, error) {
	f.lastWS = ws
	return capability.MCPContacts{Data: []capability.MCPContact{f.contact}}, nil
}
func (f *fakeReader) ReadDeal(_ context.Context, ws, id uuid.UUID) (capability.MCPDeal, error) {
	f.lastWS = ws
	return capability.MCPDeal{ID: id, Title: "x"}, nil
}
func (f *fakeReader) ListDeals(_ context.Context, ws uuid.UUID, _ store.Page) (capability.MCPDeals, error) {
	f.lastWS = ws
	return capability.MCPDeals{Data: []capability.MCPDeal{}}, nil
}
func (f *fakeReader) ListPipelineStages(_ context.Context, ws uuid.UUID) ([]capability.MCPStage, error) {
	f.lastWS = ws
	return []capability.MCPStage{{Name: "Discovery", OrderIndex: 1, DealCount: 3}}, nil
}
func (f *fakeReader) SearchContacts(_ context.Context, ws uuid.UUID, _ string) ([]capability.MCPContact, error) {
	f.lastWS = ws
	return []capability.MCPContact{f.contact}, nil
}
func (f *fakeReader) WorkspaceSchema(_ context.Context, ws uuid.UUID) (capability.MCPWorkspaceSchema, error) {
	f.lastWS = ws
	s := f.schema
	s.WorkspaceID = ws.String()
	if s.Contact == nil {
		s.Contact = []capability.MCPPropertyDef{}
	}
	if s.Deal == nil {
		s.Deal = []capability.MCPPropertyDef{}
	}
	return s, nil
}

func newTestServer(r store.Reader) *Server {
	return New(Config{Reader: r, Limiter: ratelimit.New(1000, 1000)})
}

const testWS = "11111111-1111-1111-1111-111111111111"

func rpc(t *testing.T, srv *Server, ws, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
	if ws != "" {
		req.Header.Set(workspaceHeader, ws)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func decodeResp(t *testing.T, rec *httptest.ResponseRecorder) response {
	t.Helper()
	var resp response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return resp
}

func TestInitializeHandshake(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["protocolVersion"] != protocolVersion {
		t.Fatalf("protocolVersion = %v", res["protocolVersion"])
	}
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Fatal("server must advertise tools capability")
	}
}

func TestNotificationGets202NoBody(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("notification must have empty body, got %q", rec.Body.String())
	}
}

func TestToolsListReturnsAllSixTools(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	resp := decodeResp(t, rec)
	res := resp.Result.(map[string]any)
	tools := res["tools"].([]any)
	if len(tools) != 6 {
		t.Fatalf("got %d tools, want 6", len(tools))
	}
	want := map[string]bool{
		toolReadContact: false, toolListContacts: false, toolReadDeal: false,
		toolListDeals: false, toolListPipelineStages: false, toolSearchContacts: false,
	}
	for _, tl := range tools {
		name := tl.(map[string]any)["name"].(string)
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected tool %q", name)
		}
		want[name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestToolsCall_ReadContact_PassesWorkspace(t *testing.T) {
	fr := &fakeReader{contact: capability.MCPContact{FirstName: "Ada", LastName: "Lovelace"}}
	srv := newTestServer(fr)
	id := uuid.New()
	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_contact","arguments":{"id":"` + id.String() + `"}}}`
	rec := rpc(t, srv, testWS, body)
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["isError"] == true {
		t.Fatalf("tool reported error: %v", res["content"])
	}
	// The fake must have been scoped to the header workspace.
	if fr.lastWS != uuid.MustParse(testWS) {
		t.Fatalf("reader called with ws %s, want %s", fr.lastWS, testWS)
	}
	// Content text is the JSON-encoded contact.
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !bytes.Contains([]byte(text), []byte("Lovelace")) {
		t.Fatalf("content missing contact data: %s", text)
	}
}

func TestToolsCall_NotFoundIsToolError(t *testing.T) {
	srv := newTestServer(&fakeReader{notFound: true})
	id := uuid.New()
	body := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"read_contact","arguments":{"id":"` + id.String() + `"}}}`
	rec := rpc(t, srv, testWS, body)
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("not-found should be a tool error, not transport error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["isError"] != true {
		t.Fatal("expected isError=true for not-found")
	}
}

func TestToolsCall_UnknownToolIsToolError(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	body := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"drop_table","arguments":{}}}`
	rec := rpc(t, srv, testWS, body)
	resp := decodeResp(t, rec)
	res := resp.Result.(map[string]any)
	if res["isError"] != true {
		t.Fatal("unknown tool must yield isError result")
	}
}

func TestUnknownMethodIsMethodNotFound(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":6,"method":"prompts/list"}`)
	resp := decodeResp(t, rec)
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("want method-not-found, got %+v", resp.Error)
	}
}

func TestMissingWorkspaceHeaderRejected(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, "", `{"jsonrpc":"2.0","id":7,"method":"tools/list"}`)
	resp := decodeResp(t, rec)
	if resp.Error == nil || resp.Error.Code != codeInvalidRequest {
		t.Fatalf("missing workspace header must be rejected, got %+v", resp.Error)
	}
}

func TestBadJSONIsParseError(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	rec := rpc(t, srv, testWS, `{not json`)
	resp := decodeResp(t, rec)
	if resp.Error == nil || resp.Error.Code != codeParseError {
		t.Fatalf("want parse error, got %+v", resp.Error)
	}
}

func TestGetMethodNotAllowed(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET should be 405, got %d", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(&fakeReader{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("healthz status %d", rec.Code)
	}
}

func TestRateLimitExceeded(t *testing.T) {
	srv := New(Config{Reader: &fakeReader{}, Limiter: ratelimit.New(0.0001, 1)})
	// First call consumes the only token; second is limited.
	_ = rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	resp := decodeResp(t, rec)
	if resp.Error == nil {
		t.Fatal("expected rate-limit error on 2nd call")
	}
}
