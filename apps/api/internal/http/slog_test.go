package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	httpserver "github.com/gbconsult/lecrm/apps/api/internal/http"
	"github.com/gbconsult/lecrm/apps/api/internal/logging"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

type testResolver struct {
	id       uuid.UUID
	roleName string
}

func (r *testResolver) WorkspaceBySlugFull(_ context.Context, _ string) (uuid.UUID, string, error) {
	return r.id, r.roleName, nil
}

func TestSlogMiddleware_StructuredJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	router := httpserver.NewRouter(httpserver.RouterDeps{
		Logger:          logger,
		AuthHandler:     &auth.Handler{DomainTLD: "lecrm.test"},
		CookieDomainTLD: "lecrm.test",
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rr.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nraw: %s", err, buf.String())
	}

	for _, field := range []string{"request_id", "method", "path", "status", "ms"} {
		if _, ok := entry[field]; !ok {
			t.Errorf("missing field %q in log output", field)
		}
	}

	if entry["method"] != "GET" {
		t.Errorf("method=%v want GET", entry["method"])
	}
	if entry["path"] != "/healthz" {
		t.Errorf("path=%v want /healthz", entry["path"])
	}
}

func TestSlogMiddleware_WorkspaceFieldsPresent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	id := uuid.New()
	resolver := &testResolver{id: id, roleName: "workspace_test"}

	// Use a minimal handler that doesn't need a DB pool.
	wsHandler := &workspace.TestListHandler{Logger: logger}
	_ = wsHandler // used indirectly below

	router := httpserver.NewRouter(httpserver.RouterDeps{
		Logger:          logger,
		AuthHandler:     &auth.Handler{DomainTLD: "lecrm.test"},
		Resolver:        resolver,
		TestList:        wsHandler,
		CookieDomainTLD: "lecrm.test",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/_test/workspaces", nil)
	req.Host = "acme.lecrm.test:8080"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// The handler panics (nil pool) but chi Recoverer catches it and the
	// slog middleware still emits a completion log with workspace fields.
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) == 0 {
		t.Fatal("no log output")
	}

	var httpEntry map[string]any
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if msg, _ := entry["msg"].(string); msg == "http" {
			httpEntry = entry
			break
		}
	}

	if httpEntry == nil {
		t.Fatalf("no 'http' log line found in output:\n%s", buf.String())
	}

	if httpEntry["workspace"] != "acme" {
		t.Errorf("workspace=%v want acme", httpEntry["workspace"])
	}
	if httpEntry["workspace_id"] != id.String() {
		t.Errorf("workspace_id=%v want %s", httpEntry["workspace_id"], id.String())
	}
	if _, ok := httpEntry["request_id"]; !ok {
		t.Error("missing request_id field")
	}
}

func TestLoggingContext_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := logging.WithLogger(context.Background(), logger)

	got := logging.FromContext(ctx)
	if got != logger {
		t.Error("FromContext did not return the stored logger")
	}

	defaultLogger := logging.FromContext(context.Background())
	if defaultLogger == nil {
		t.Error("FromContext returned nil for empty context")
	}
}
