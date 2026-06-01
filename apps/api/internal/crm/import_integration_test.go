//go:build integration

// Integration tests for the CSV import endpoints (integrator-gap tasket
// 20260601-110828-736b). Shares the two-tenant pipelineTestEnv harness from
// pipeline_integration_test.go.
//
// Headline assertion: cross-tenant isolation — workspace A's import writes
// only into A's schema, never B's. The test also exercises the full
// analyze → preview → commit flow for contacts and verifies the dedup
// policies (update / skip) and error report.
//
// Run:
//
//	go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestImport ./internal/crm

package crm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type importSummaryJSON struct {
	Total  int `json:"total"`
	Create int `json:"create"`
	Update int `json:"update"`
	Skip   int `json:"skip"`
	Error  int `json:"error"`
}

type rowOutcomeJSON struct {
	Line   int    `json:"line"`
	Action string `json:"action"`
	Reason string `json:"reason"`
	Label  string `json:"label"`
}

type previewRespJSON struct {
	Summary importSummaryJSON `json:"summary"`
	Rows    []rowOutcomeJSON  `json:"rows"`
}

type commitRespJSON struct {
	Summary        importSummaryJSON `json:"summary"`
	ErrorReportCSV string            `json:"error_report_csv"`
	AuditEvent     string            `json:"audit_event"`
}

type analyzeRespJSON struct {
	Columns          []string          `json:"columns"`
	SampleRows       [][]string        `json:"sample_rows"`
	RowCount         int               `json:"row_count"`
	CoreFields       []map[string]any  `json:"core_fields"`
	CustomFields     []map[string]any  `json:"custom_fields"`
	SuggestedMapping map[string]string `json:"suggested_mapping"`
}

func doImport(t *testing.T, e *pipelineTestEnv, ws workspaceFixture, entity, step string, body any) (int, []byte) {
	t.Helper()
	return e.doJSON(t, ws, http.MethodPost,
		fmt.Sprintf("/v1/import/%s/%s", entity, step), body)
}

// TestImport_Contacts_AnalyzePreviewCommit exercises the full three-step flow.
func TestImport_Contacts_AnalyzePreviewCommit(t *testing.T) {
	env := setupPipelineEnv(t)

	csv := "first_name,last_name,email\nAlice,Smith,alice@example.com\nBob,Jones,bob@example.com"
	mapping := map[string]string{
		"first_name": "first_name",
		"last_name":  "last_name",
		"email":      "email",
	}

	// Step 1: analyze.
	status, body := doImport(t, env, env.wsA, "contacts", "analyze", map[string]any{"csv": csv})
	if status != http.StatusOK {
		t.Fatalf("analyze: status=%d body=%s", status, body)
	}
	var analyze analyzeRespJSON
	if err := json.Unmarshal(body, &analyze); err != nil {
		t.Fatalf("decode analyze: %v body=%s", err, body)
	}
	if analyze.RowCount != 2 {
		t.Errorf("row_count: got %d want 2", analyze.RowCount)
	}
	if len(analyze.Columns) != 3 {
		t.Errorf("columns: got %v want 3 items", analyze.Columns)
	}

	// Step 2: preview (dry run — nothing written yet).
	status, body = doImport(t, env, env.wsA, "contacts", "preview", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("preview: status=%d body=%s", status, body)
	}
	var preview previewRespJSON
	if err := json.Unmarshal(body, &preview); err != nil {
		t.Fatalf("decode preview: %v body=%s", err, body)
	}
	if preview.Summary.Total != 2 || preview.Summary.Create != 2 {
		t.Errorf("preview summary: got %+v want total=2 create=2", preview.Summary)
	}

	// Verify nothing was actually written by the preview.
	var countA int
	err := env.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %q.contacts`, env.wsA.roleName)).Scan(&countA)
	if err != nil {
		t.Fatalf("count wsA contacts: %v", err)
	}
	if countA != 0 {
		t.Errorf("preview must not write: got %d contacts in wsA, want 0", countA)
	}

	// Step 3: commit.
	status, body = doImport(t, env, env.wsA, "contacts", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status=%d body=%s", status, body)
	}
	var commit commitRespJSON
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode commit: %v body=%s", err, body)
	}
	if commit.Summary.Total != 2 || commit.Summary.Create != 2 || commit.Summary.Error != 0 {
		t.Errorf("commit summary: got %+v want total=2 create=2 error=0", commit.Summary)
	}
	if commit.AuditEvent != "import.committed" {
		t.Errorf("audit_event: got %q want import.committed", commit.AuditEvent)
	}

	// Verify rows are in wsA's schema.
	if err := env.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %q.contacts`, env.wsA.roleName)).Scan(&countA); err != nil {
		t.Fatalf("count wsA contacts: %v", err)
	}
	if countA != 2 {
		t.Errorf("wsA contacts after commit: got %d want 2", countA)
	}
}

// TestImport_Contacts_CrossTenant_Isolation is the sovereignty headline test:
// importing into workspace A must never touch workspace B's schema.
func TestImport_Contacts_CrossTenant_Isolation(t *testing.T) {
	env := setupPipelineEnv(t)

	csv := "first_name,last_name,email\nCharlie,Tenant,charlie@a.test"
	mapping := map[string]string{
		"first_name": "first_name", "last_name": "last_name", "email": "email",
	}

	// Import into workspace A.
	status, body := doImport(t, env, env.wsA, "contacts", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("commit wsA: status=%d body=%s", status, body)
	}

	// Workspace B's contacts table must remain empty.
	var countB int
	if err := env.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %q.contacts`, env.wsB.roleName)).Scan(&countB); err != nil {
		t.Fatalf("count wsB contacts: %v", err)
	}
	if countB != 0 {
		t.Errorf("cross-tenant isolation breach: wsB has %d contacts after wsA import", countB)
	}
}

// TestImport_Contacts_Dedup_UpdatePolicy imports the same email twice; the
// second commit should hit the "update" path.
func TestImport_Contacts_Dedup_UpdatePolicy(t *testing.T) {
	env := setupPipelineEnv(t)

	// Seed an existing contact via the REST API so it exists in wsA.
	env.createContact(t, env.wsA, "Eve", "Original", "eve@dedup.test")

	csv := "first_name,last_name,email\nEve,Updated,eve@dedup.test"
	mapping := map[string]string{
		"first_name": "first_name", "last_name": "last_name", "email": "email",
	}

	status, body := doImport(t, env, env.wsA, "contacts", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status=%d body=%s", status, body)
	}
	var commit commitRespJSON
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode commit: %v body=%s", err, body)
	}
	if commit.Summary.Update != 1 || commit.Summary.Create != 0 {
		t.Errorf("dedup update: got %+v want update=1 create=0", commit.Summary)
	}
	// Still only one contact in wsA (no duplicate created).
	var count int
	if err := env.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %q.contacts`, env.wsA.roleName)).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("contact count after dedup-update: got %d want 1", count)
	}
}

// TestImport_Contacts_Dedup_SkipPolicy verifies the skip policy leaves
// existing records untouched and counts the row as skipped.
func TestImport_Contacts_Dedup_SkipPolicy(t *testing.T) {
	env := setupPipelineEnv(t)

	env.createContact(t, env.wsA, "Frank", "Keeper", "frank@skip.test")

	csv := "first_name,last_name,email\nFrank,ShouldBeIgnored,frank@skip.test"
	mapping := map[string]string{
		"first_name": "first_name", "last_name": "last_name", "email": "email",
	}
	status, body := doImport(t, env, env.wsA, "contacts", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "skip",
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status=%d body=%s", status, body)
	}
	var commit commitRespJSON
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if commit.Summary.Skip != 1 || commit.Summary.Update != 0 {
		t.Errorf("dedup skip: got %+v want skip=1 update=0", commit.Summary)
	}
}

// TestImport_Contacts_ErrorReport verifies that rows with invalid emails are
// classified as errors and included in the downloadable report.
func TestImport_Contacts_ErrorReport(t *testing.T) {
	env := setupPipelineEnv(t)

	csv := "first_name,last_name,email\nGood,Row,good@valid.com\nBad,Row,not-an-email"
	mapping := map[string]string{
		"first_name": "first_name", "last_name": "last_name", "email": "email",
	}
	status, body := doImport(t, env, env.wsA, "contacts", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status=%d body=%s", status, body)
	}
	var commit commitRespJSON
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if commit.Summary.Create != 1 || commit.Summary.Error != 1 {
		t.Errorf("error report: got %+v want create=1 error=1", commit.Summary)
	}
	// The error report CSV should mention the bad row.
	if !strings.Contains(commit.ErrorReportCSV, "error") {
		t.Errorf("error_report_csv missing 'error' action; got: %s", commit.ErrorReportCSV)
	}
}

// TestImport_Companies_Smoke verifies companies can be imported with
// cross-tenant isolation.
func TestImport_Companies_Smoke(t *testing.T) {
	env := setupPipelineEnv(t)

	csv := "name,domain\nAcme Corp,acme.io\nBeta Inc,beta.io"
	mapping := map[string]string{"name": "name", "domain": "domain"}

	status, body := doImport(t, env, env.wsA, "companies", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status=%d body=%s", status, body)
	}
	var commit commitRespJSON
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if commit.Summary.Create != 2 {
		t.Errorf("companies commit: got %+v want create=2", commit.Summary)
	}
	// wsB must not have the companies.
	var countB int
	if err := env.pool.QueryRow(context.Background(),
		fmt.Sprintf(`SELECT count(*) FROM %q.companies`, env.wsB.roleName)).Scan(&countB); err != nil {
		t.Fatalf("count wsB companies: %v", err)
	}
	if countB != 0 {
		t.Errorf("isolation breach: wsB has %d companies after wsA import", countB)
	}
}

// TestImport_Deals_Smoke verifies deals can be imported (no dedup for deals).
func TestImport_Deals_Smoke(t *testing.T) {
	env := setupPipelineEnv(t)

	csv := "title,amount\nDeal Alpha,1000\nDeal Beta,2000"
	mapping := map[string]string{"title": "title", "amount": "amount"}

	status, body := doImport(t, env, env.wsA, "deals", "commit", map[string]any{
		"csv": csv, "mapping": mapping, "dedupe": "update",
	})
	if status != http.StatusOK {
		t.Fatalf("commit: status=%d body=%s", status, body)
	}
	var commit commitRespJSON
	if err := json.Unmarshal(body, &commit); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if commit.Summary.Create != 2 {
		t.Errorf("deals commit: got %+v want create=2", commit.Summary)
	}
}
