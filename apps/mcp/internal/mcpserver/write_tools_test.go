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

// fakeWriter records the inputs the JSON-RPC layer forwards and returns a
// canned WriteResult, so dispatch/argument-mapping can be tested without a
// database. The real scope→role gate is exercised at the capability layer
// (capability.TestMCPWritePrincipal_ScopeGate); here we assert the MCP layer
// forwards the workspace, scopes, and parsed intent faithfully and maps the
// result (preview vs body vs error) correctly.
type fakeWriter struct {
	lastWS     uuid.UUID
	lastScopes []string
	lastAdvIn  capability.AdvanceDealInput
	lastLogIn  capability.LogInteractionInput
	lastCapIn  capability.CaptureLeadInput
	lastOpts   capability.WriteOptions

	result capability.WriteResult
	err    error
}

func (f *fakeWriter) AdvanceDeal(_ context.Context, ws uuid.UUID, scopes []string, in capability.AdvanceDealInput, opts capability.WriteOptions) (capability.WriteResult, error) {
	f.lastWS, f.lastScopes, f.lastAdvIn, f.lastOpts = ws, scopes, in, opts
	return f.result, f.err
}
func (f *fakeWriter) LogInteraction(_ context.Context, ws uuid.UUID, scopes []string, in capability.LogInteractionInput, opts capability.WriteOptions) (capability.WriteResult, error) {
	f.lastWS, f.lastScopes, f.lastLogIn, f.lastOpts = ws, scopes, in, opts
	return f.result, f.err
}
func (f *fakeWriter) CaptureLead(_ context.Context, ws uuid.UUID, scopes []string, in capability.CaptureLeadInput, opts capability.WriteOptions) (capability.WriteResult, error) {
	f.lastWS, f.lastScopes, f.lastCapIn, f.lastOpts = ws, scopes, in, opts
	return f.result, f.err
}

func newWriteServer(r store.Reader, w store.Writer) *Server {
	return New(Config{Reader: r, Writer: w, Limiter: ratelimit.New(1000, 1000)})
}

// rpcScoped is like rpc but stamps the granted-scopes header the gateway sets.
func rpcScoped(t *testing.T, srv *Server, ws, scopes, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
	req.Header.Set(workspaceHeader, ws)
	if scopes != "" {
		req.Header.Set(scopesHeader, scopes)
	}
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func toolResult(t *testing.T, resp response) map[string]any {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected transport error: %+v", resp.Error)
	}
	return resp.Result.(map[string]any)
}

// toolText returns the decoded JSON object the tool emitted as its text block.
func toolText(t *testing.T, res map[string]any) map[string]any {
	t.Helper()
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("tool text is not JSON: %v; text=%s", err, text)
	}
	return out
}

func TestToolsList_IncludesWriteToolsWhenWriterConfigured(t *testing.T) {
	srv := newWriteServer(&fakeReader{}, &fakeWriter{})
	rec := rpcScoped(t, srv, testWS, "crm:write", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res := toolResult(t, decodeResp(t, rec))
	tools := res["tools"].([]any)
	if len(tools) != 9 {
		t.Fatalf("got %d tools, want 9 (6 read + 3 write)", len(tools))
	}
	want := map[string]bool{toolAdvanceDeal: false, toolLogInteraction: false, toolCaptureLead: false}
	for _, tl := range tools {
		name := tl.(map[string]any)["name"].(string)
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("write tool %q missing from catalog", name)
		}
	}
}

func TestToolsList_OmitsWriteToolsWhenReadOnly(t *testing.T) {
	srv := newTestServer(&fakeReader{}) // no writer
	rec := rpc(t, srv, testWS, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	res := toolResult(t, decodeResp(t, rec))
	if n := len(res["tools"].([]any)); n != 6 {
		t.Fatalf("read-only catalog must have 6 tools, got %d", n)
	}
}

func TestAdvanceDeal_ForwardsWorkspaceScopesAndArgs(t *testing.T) {
	fw := &fakeWriter{result: capability.WriteResult{Status: 200, Body: []byte(`{"id":"d1","title":"Acme"}`)}}
	srv := newWriteServer(&fakeReader{}, fw)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"advance_deal","arguments":{"deal":"Acme","to_stage":"won","mark_closed_at":true,"idempotency_key":"k-1"}}}`
	rec := rpcScoped(t, srv, testWS, "crm:write deals:write", body)
	res := toolResult(t, decodeResp(t, rec))
	if res["isError"] == true {
		t.Fatalf("unexpected tool error: %v", res["content"])
	}
	if fw.lastWS != uuid.MustParse(testWS) {
		t.Fatalf("ws = %s, want %s", fw.lastWS, testWS)
	}
	if len(fw.lastScopes) != 2 || fw.lastScopes[0] != "crm:write" || fw.lastScopes[1] != "deals:write" {
		t.Fatalf("scopes = %v, want [crm:write deals:write]", fw.lastScopes)
	}
	if fw.lastAdvIn.Deal != "Acme" || fw.lastAdvIn.ToStage != "won" {
		t.Fatalf("intent not forwarded: %+v", fw.lastAdvIn)
	}
	if fw.lastAdvIn.MarkClosedAt == nil || *fw.lastAdvIn.MarkClosedAt != "now" {
		t.Fatalf("mark_closed_at:true must map to *\"now\", got %v", fw.lastAdvIn.MarkClosedAt)
	}
	if fw.lastOpts.IdempotencyKey != "k-1" {
		t.Fatalf("idempotency_key not forwarded: %q", fw.lastOpts.IdempotencyKey)
	}
	out := toolText(t, res)
	if out["title"] != "Acme" {
		t.Fatalf("body not surfaced: %v", out)
	}
}

func TestAdvanceDeal_DryRunReturnsPreview(t *testing.T) {
	fw := &fakeWriter{result: capability.WriteResult{Preview: &capability.Preview{
		DryRun: true, Operation: "advance_deal", Summary: "would advance",
	}}}
	srv := newWriteServer(&fakeReader{}, fw)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"advance_deal","arguments":{"deal":"d","to_stage":"won","dry_run":true}}}`
	rec := rpcScoped(t, srv, testWS, "crm:write", body)
	res := toolResult(t, decodeResp(t, rec))
	if !fw.lastOpts.DryRun {
		t.Fatal("dry_run flag not forwarded to WriteOptions")
	}
	out := toolText(t, res)
	if out["dry_run"] != true || out["operation"] != "advance_deal" {
		t.Fatalf("preview not surfaced: %v", out)
	}
}

func TestCaptureLead_ForwardsArgs(t *testing.T) {
	fw := &fakeWriter{result: capability.WriteResult{Status: 201, Body: []byte(`{"contact_id":"c1","deal_id":"d1"}`)}}
	srv := newWriteServer(&fakeReader{}, fw)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"capture_lead","arguments":{"name":"Ada Lovelace","email":"ada@x.io","company":"Analytical","source":"web-chat"}}}`
	rec := rpcScoped(t, srv, testWS, "crm:write", body)
	res := toolResult(t, decodeResp(t, rec))
	if res["isError"] == true {
		t.Fatalf("unexpected error: %v", res["content"])
	}
	if fw.lastCapIn.Name != "Ada Lovelace" || fw.lastCapIn.Source != "web-chat" {
		t.Fatalf("intent not forwarded: %+v", fw.lastCapIn)
	}
	if fw.lastCapIn.Email == nil || *fw.lastCapIn.Email != "ada@x.io" {
		t.Fatalf("email not forwarded: %v", fw.lastCapIn.Email)
	}
	if fw.lastCapIn.Company == nil || *fw.lastCapIn.Company != "Analytical" {
		t.Fatalf("company not forwarded: %v", fw.lastCapIn.Company)
	}
}

// A read-only deployment (no writer) must reject the write tools as a tool
// error, never silently — and never reach a writer.
func TestWriteTool_DisabledWhenReadOnly(t *testing.T) {
	srv := newTestServer(&fakeReader{}) // no writer
	for _, name := range []string{toolAdvanceDeal, toolLogInteraction, toolCaptureLead} {
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":{"name":"x","source":"s","deal":"d","to_stage":"w","contact_or_company":"c","summary":"s"}}}`
		rec := rpc(t, srv, testWS, body)
		res := toolResult(t, decodeResp(t, rec))
		if res["isError"] != true {
			t.Fatalf("%s on read-only server must be an isError result", name)
		}
	}
}

// When the capability gate denies a write (e.g. read-only token), the denial
// surfaces to the agent as an isError tool result, not a transport error.
func TestWriteTool_GateDenialIsToolError(t *testing.T) {
	fw := &fakeWriter{err: capability.ErrReadOnlyScope}
	srv := newWriteServer(&fakeReader{}, fw)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"capture_lead","arguments":{"name":"Ada","source":"chat"}}}`
	rec := rpcScoped(t, srv, testWS, "crm:read", body)
	resp := decodeResp(t, rec)
	if resp.Error != nil {
		t.Fatalf("denial must be a tool error, not transport: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["isError"] != true {
		t.Fatal("read-only denial must be isError=true")
	}
	// The scopes still reached the writer (so the gate could fire).
	if len(fw.lastScopes) != 1 || fw.lastScopes[0] != "crm:read" {
		t.Fatalf("scopes not forwarded for the gate: %v", fw.lastScopes)
	}
}

func TestParseFlexClosedAt(t *testing.T) {
	mk := func(s string) json.RawMessage { return json.RawMessage(s) }
	if parseFlexClosedAt(nil) != nil {
		t.Fatal("absent ⇒ nil")
	}
	if parseFlexClosedAt(mk(`null`)) != nil {
		t.Fatal("null ⇒ nil")
	}
	if parseFlexClosedAt(mk(`false`)) != nil {
		t.Fatal("false ⇒ nil (no close requested)")
	}
	if got := parseFlexClosedAt(mk(`true`)); got == nil || *got != "now" {
		t.Fatalf("true ⇒ *\"now\", got %v", got)
	}
	if got := parseFlexClosedAt(mk(`"2026-01-15"`)); got == nil || *got != "2026-01-15" {
		t.Fatalf("date string passthrough, got %v", got)
	}
	if got := parseFlexClosedAt(mk(`"today"`)); got == nil || *got != "today" {
		t.Fatalf("\"today\" passthrough, got %v", got)
	}
}
