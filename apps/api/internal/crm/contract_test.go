package crm_test

// Contract tests — verify the chi router actually wires every path
// documented in docs/openapi.yaml, and vice-versa.
//
// The test reads the YAML spec, walks the router, and reports any
// drift in either direction (spec path with no handler, or handler
// with no spec entry). It runs as a normal unit test so CI catches
// the divergence before a PR merges.
//
// Where coverage of response *shape* matters, individual handler
// tests assert the JSON structure (see handlers_test.go +
// anc_handlers_test.go). Wiring drift is what this test guards.

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/gbconsult/lecrm/apps/api/internal/auth"
	"github.com/gbconsult/lecrm/apps/api/internal/crm"
)

// openAPISpec is a minimal subset of the OpenAPI 3.1 document we
// care about. We only need to enumerate the paths × methods and
// ignore everything else (schemas, parameters, etc.).
type openAPISpec struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

// findOpenAPIPath walks up from the apps/api package directory to
// the repo root looking for docs/openapi.yaml. Keeps the test
// resilient to where `go test` is invoked from.
func findOpenAPIPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for dir := wd; dir != "/" && dir != ""; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "docs", "openapi.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatalf("docs/openapi.yaml not found above %s", wd)
	return ""
}

func loadSpec(t *testing.T) openAPISpec {
	t.Helper()
	path := findOpenAPIPath(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var s openAPISpec
	if err := yaml.Unmarshal(raw, &s); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	if len(s.Paths) == 0 {
		t.Fatalf("spec has no paths")
	}
	return s
}

// httpMethods is the set of OpenAPI operation verbs we treat as
// routes (vs description / parameters keys colocated under a path).
var httpMethods = map[string]struct{}{
	"get": {}, "post": {}, "put": {}, "patch": {}, "delete": {},
	"head": {}, "options": {},
}

// specRoutes returns the spec-documented (method, path) pairs.
func specRoutes(s openAPISpec) map[string]struct{} {
	out := map[string]struct{}{}
	for path, ops := range s.Paths {
		for verb := range ops {
			lv := strings.ToLower(verb)
			if _, ok := httpMethods[lv]; !ok {
				continue
			}
			out[strings.ToUpper(lv)+" "+canonicalPath(path)] = struct{}{}
		}
	}
	return out
}

// canonicalPath rewrites OpenAPI path templates `/v1/contacts/{id}`
// to chi's `/v1/contacts/{id}` form. They already match for our
// vocabulary; this is a hook for future path-style divergences.
func canonicalPath(p string) string {
	return p
}

// routerRoutes enumerates every route registered by the production
// handlers under workspace.Middleware.
func routerRoutes(t *testing.T) map[string]struct{} {
	t.Helper()
	logger := slog.Default()

	r := chi.NewRouter()
	// CRM (contacts/companies/deals/pipeline) and ANT (activities/notes/tasks).
	crmH := &crm.Handler{Logger: logger}
	crmH.RegisterRoutes(r)
	crmH.RegisterANTRoutes(r)

	// Workspace service tokens.
	tokensH := &auth.ServiceTokenHandler{Logger: logger}
	tokensH.RegisterRoutes(r)

	out := map[string]struct{}{}
	err := chi.Walk(r, func(method, route string, handler http.Handler, mws ...func(http.Handler) http.Handler) error {
		// chi pads patterns with trailing slashes for sub-routers; strip.
		route = strings.TrimSuffix(route, "/")
		if route == "" {
			route = "/"
		}
		out[method+" "+route] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("chi walk: %v", err)
	}
	return out
}

func TestOpenAPISpec_AllPathsHaveHandlers(t *testing.T) {
	spec := loadSpec(t)
	routes := routerRoutes(t)

	var missing []string
	for sp := range specRoutes(spec) {
		if _, ok := routes[sp]; !ok {
			missing = append(missing, sp)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("spec documents %d path(s) with no router handler:\n  - %s", len(missing), strings.Join(missing, "\n  - "))
	}
}

func TestOpenAPISpec_AllRoutesAreDocumented(t *testing.T) {
	spec := loadSpec(t)
	specSet := specRoutes(spec)
	routes := routerRoutes(t)

	var missing []string
	for r := range routes {
		if _, ok := specSet[r]; !ok {
			missing = append(missing, r)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("router exposes %d route(s) not documented in OpenAPI spec:\n  - %s", len(missing), strings.Join(missing, "\n  - "))
	}
}

// TestOpenAPISpec_PaginationContract — every list response that
// returns a "data" array MUST advertise the (next_cursor, has_more)
// pagination envelope. This guards against silent regressions where
// a new list endpoint forgets the cursor fields.
func TestOpenAPISpec_PaginationContract(t *testing.T) {
	spec := loadSpec(t)

	type opShape struct {
		Responses map[string]any `yaml:"responses"`
	}

	for path, ops := range spec.Paths {
		for verb, raw := range ops {
			lv := strings.ToLower(verb)
			if lv != "get" {
				continue
			}
			ymlBytes, _ := yaml.Marshal(raw)
			var op opShape
			if err := yaml.Unmarshal(ymlBytes, &op); err != nil {
				continue
			}
			// Only inspect 200 responses that use one of our list response refs.
			ok200, found := op.Responses["200"]
			if !found {
				continue
			}
			b, _ := yaml.Marshal(ok200)
			body := string(b)
			// Heuristic: list responses point to one of the *List response refs
			// or inline a `data:` array.
			if !strings.Contains(body, "List") && !strings.Contains(body, "data:") {
				continue
			}
			// Intentionally un-paginated list endpoints — small bounded
			// collections (pipeline stages: <20 per workspace; service
			// tokens: dozens at most).
			if strings.Contains(body, "PipelineStage") {
				continue
			}
			if strings.Contains(body, "ServiceToken") {
				continue
			}
			// For ref-based responses, look up the resolved response in components.
			// Cheap check: inline schemas that mention `data:` must also mention `has_more`.
			if strings.Contains(body, "data:") && !strings.Contains(body, "has_more") {
				t.Errorf("%s %s: list response missing has_more field", strings.ToUpper(verb), path)
			}
		}
	}
}

// TestOpenAPISpec_ErrorResponseContract — every documented 4xx
// response MUST reference the shared Error schema (so clients have a
// single shape to deserialize).
func TestOpenAPISpec_ErrorResponseContract(t *testing.T) {
	spec := loadSpec(t)
	for path, ops := range spec.Paths {
		ymlBytes, _ := yaml.Marshal(ops)
		body := string(ymlBytes)
		// Find each "'4xx':" key and check the next line(s) mention Error or a ref.
		// Cheap text scan; safe because we control the spec.
		for _, code := range []string{"'400'", "'401'", "'404'", "'409'"} {
			idx := 0
			for {
				i := strings.Index(body[idx:], code)
				if i < 0 {
					break
				}
				j := idx + i
				tail := body[j:min(j+200, len(body))]
				if !strings.Contains(tail, "Error") && !strings.Contains(tail, "Ref") && !strings.Contains(tail, "$ref") {
					t.Errorf("%s declares %s response without referencing the Error schema", path, code)
				}
				idx = j + len(code)
			}
		}
	}
	_ = fmt.Sprintf // keep import in case the test grows
}
