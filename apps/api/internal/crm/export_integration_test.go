//go:build integration

// Integration tests for the CSV export endpoints (Sprint 9, feature 8).
//
// Reuses the two-tenant testcontainers harness from
// pipeline_integration_test.go (setupPipelineEnv). The headline assertion
// is cross-tenant isolation: workspace A's export must contain only A's
// rows, never B's — the export query runs inside A's own schema via the
// workspace middleware's search_path, so a leak would be a sovereignty
// breach (ADR-009 §1).
//
// Run:
//
//	go -C apps/api test -tags integration -count 1 -race -run TestExport ./internal/crm

package crm_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// doRaw issues a GET as the given workspace and returns status, headers, body.
func (e *pipelineTestEnv) doRaw(t *testing.T, ws workspaceFixture, path string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := ws.client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(body)
}

func parseCSV(t *testing.T, body string) ([]string, [][]string) {
	t.Helper()
	r := csv.NewReader(strings.NewReader(body))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v; body=%q", err, body)
	}
	if len(records) == 0 {
		t.Fatal("CSV has no rows (missing header)")
	}
	return records[0], records[1:]
}

func TestExport_Contacts_Isolation(t *testing.T) {
	env := setupPipelineEnv(t)

	aID := env.createContact(t, env.wsA, "Ada", "Lovelace", "ada@a.test").ID
	env.createContact(t, env.wsB, "Brian", "Kernighan", "brian@b.test")

	// Attach a custom property to A's contact (insert directly; export reads
	// the objects table, no definition needed to surface it).
	insertSQL := fmt.Sprintf(
		`INSERT INTO %q.objects (object_type, parent_type, parent_id, data)
		 VALUES ('custom_properties', 'contact', $1, $2)`, env.wsA.roleName)
	_, err := env.pool.Exec(context.Background(), insertSQL, aID, []byte(`{"tier":"gold"}`))
	if err != nil {
		t.Fatalf("insert custom property: %v", err)
	}

	status, hdr, body := env.doRaw(t, env.wsA, "/v1/contacts/export")
	if status != http.StatusOK {
		t.Fatalf("export contacts: status=%d body=%s", status, body)
	}
	if ct := hdr.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if cd := hdr.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "contacts_") {
		t.Errorf("Content-Disposition = %q, want attachment contacts_<date>.csv", cd)
	}

	header, rows := parseCSV(t, body)
	if !slices.Contains(header, "cf_tier") {
		t.Errorf("header %v missing flattened custom column cf_tier", header)
	}
	if len(rows) != 1 {
		t.Fatalf("wsA export has %d data rows, want 1 (cross-tenant leak?)", len(rows))
	}
	if !strings.Contains(body, "Ada") || strings.Contains(body, "Brian") {
		t.Errorf("isolation breach: wsA export must contain Ada and not Brian; body=%s", body)
	}
	// The custom value travels in the cf_tier column.
	if !strings.Contains(body, "gold") {
		t.Errorf("custom property value 'gold' missing from export; body=%s", body)
	}
}

func TestExport_Deals_And_Companies_Smoke(t *testing.T) {
	env := setupPipelineEnv(t)

	stages := env.listStages(t, env.wsA)
	env.createDeal(t, env.wsA, "Big Deal", stages[0].ID)

	for _, ep := range []struct{ path, prefix string }{
		{"/v1/deals/export", "deals_"},
		{"/v1/companies/export", "companies_"},
	} {
		status, hdr, body := env.doRaw(t, env.wsA, ep.path)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status=%d body=%s", ep.path, status, body)
		}
		if cd := hdr.Get("Content-Disposition"); !strings.Contains(cd, ep.prefix) {
			t.Errorf("GET %s Content-Disposition = %q, want %s<date>.csv", ep.path, cd, ep.prefix)
		}
		header, _ := parseCSV(t, body)
		if len(header) == 0 {
			t.Errorf("GET %s produced no header row", ep.path)
		}
	}
}
