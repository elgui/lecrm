// Package http_test contains the endpoint-registry guard test required by
// docs/test-strategy.md §4.1 TR-3.
//
// The guard walks the chi router returned by NewRouter, collects every
// /v1/* route, and asserts each one has an entry in
// docs/test-strategy-endpoint-registry.json. A route without a registry
// entry fails CI — this forces every new tenant-scoped handler to ship
// with a declared isolation test.
//
// TR-3 pre-condition: the guard is wired now but enforcement is soft until
// ≥2 tenant-scoped handlers exist. At Sprint 3 close (1 handler), the
// single registered route has its registry entry and the test passes.
// When Sprint 4 adds a second handler without a registry entry, this test
// fails immediately.
package http_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	httpserver "github.com/gbconsult/lecrm/apps/api/internal/http"
	"github.com/gbconsult/lecrm/apps/api/internal/metadata"
	"github.com/gbconsult/lecrm/apps/api/internal/workspace"
)

// endpointRegistryEntry mirrors one item in test-strategy-endpoint-registry.json.
type endpointRegistryEntry struct {
	Method        string `json:"method"`
	Path          string `json:"path"`
	IsolationTest string `json:"isolation_test"`
}

type endpointRegistry struct {
	Version   int                     `json:"version"`
	Endpoints []endpointRegistryEntry `json:"endpoints"`
}

// registryPath locates docs/test-strategy-endpoint-registry.json relative
// to this source file.
func registryPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is apps/api/internal/http/coverage_test.go
	// Four levels up reaches the repo root.
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	p := filepath.Join(repoRoot, "docs", "test-strategy-endpoint-registry.json")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("endpoint registry not found at %s: %v", p, err)
	}
	return p
}

// TestEndpointRegistry_AllV1RoutesAreRegistered walks the chi router and
// asserts every /v1/* route has a matching entry in the endpoint registry.
//
// Failure means a new handler was added to NewRouter without declaring its
// isolation and RBAC tests — the CI gate per docs/test-strategy.md §4.1 TR-3.
func TestEndpointRegistry_AllV1RoutesAreRegistered(t *testing.T) {
	// Build router with stub dependencies. Auth handler is non-nil so
	// Register() can mount routes; we never invoke any route in this test.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	deps := httpserver.RouterDeps{
		Logger:          logger,
		AuthHandler:     &auth.Handler{DomainTLD: "lecrm.test"},
		Resolver:        &workspace.PoolResolver{},
		TestList:        &workspace.TestListHandler{},
		Metadata:        &metadata.Handler{Logger: logger},
		CookieDomainTLD: "lecrm.test",
	}
	router := httpserver.NewRouter(deps)

	// Collect all /v1/* routes from the chi router tree.
	var routerRoutes []string
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/v1/") {
			routerRoutes = append(routerRoutes, method+" "+route)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}

	// Load the registry.
	raw, err := os.ReadFile(registryPath(t))
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var registry endpointRegistry
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatalf("parse registry: %v", err)
	}

	// Index by "METHOD /path" for O(1) lookup.
	registered := make(map[string]endpointRegistryEntry, len(registry.Endpoints))
	for _, e := range registry.Endpoints {
		key := strings.ToUpper(e.Method) + " " + e.Path
		registered[key] = e
	}

	// Assert every router route appears in the registry.
	for _, route := range routerRoutes {
		entry, ok := registered[route]
		if !ok {
			t.Errorf("UNREGISTERED ROUTE: %q — add isolation_test and rbac_test entries to "+
				"docs/test-strategy-endpoint-registry.json before merging (§4.1 TR-3)", route)
			continue
		}
		if entry.IsolationTest == "" {
			t.Errorf("MISSING isolation_test for route %q in endpoint registry", route)
		}
	}

	t.Logf("endpoint registry: %d router routes checked against %d registry entries",
		len(routerRoutes), len(registry.Endpoints))
}
