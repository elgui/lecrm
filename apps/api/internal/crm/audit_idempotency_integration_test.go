//go:build integration

// Audit-log + Idempotency-Key integration tests for the CRM handlers
// (tasket 20260525-1003 residual scope, Sprint 7).
//
// Run:
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run TestAuditIdempotency ./internal/crm

package crm_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func countAuditEvents(t *testing.T, env *pipelineTestEnv, ws workspaceFixture, event string) int {
	t.Helper()
	var n int
	err := env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM core.audit_log WHERE workspace_id = $1 AND event = $2`,
		ws.id, event,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// doJSONH is doJSON with an additional headers map (specifically used
// to attach Idempotency-Key). Returns status, body, and response headers.
func (e *pipelineTestEnv) doJSONH(t *testing.T, ws workspaceFixture, method, path string, body any, headers map[string]string) (int, []byte, http.Header) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := ws.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, resp.Header
}

// --- Test 1: POST with Idempotency-Key replays the cached response ---

func TestAuditIdempotency_CreateContact_ReplaysCachedResponse(t *testing.T) {
	env := setupPipelineEnv(t)
	key := "tasket-1003-key-" + uuid.New().String()

	st1, body1, hdr1 := env.doJSONH(t, env.wsA, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": "Ada", "last_name": "Lovelace", "email": "ada@example.test"},
		map[string]string{"Idempotency-Key": key},
	)
	if st1 != http.StatusCreated {
		t.Fatalf("first POST status: got %d want 201, body=%s", st1, body1)
	}
	if hdr1.Get("Idempotency-Replayed") != "" {
		t.Error("first POST must not carry Idempotency-Replayed header")
	}

	// Second POST with the SAME key but a DIFFERENT payload — replay
	// returns the cached (first) body, not a fresh result for the new
	// payload. That's the whole point of idempotency.
	st2, body2, hdr2 := env.doJSONH(t, env.wsA, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": "Different", "last_name": "Payload", "email": "diff@example.test"},
		map[string]string{"Idempotency-Key": key},
	)
	if st2 != st1 {
		t.Errorf("replay status: got %d want %d", st2, st1)
	}
	if string(body2) != string(body1) {
		t.Errorf("replay body mismatch:\n got %s\nwant %s", body2, body1)
	}
	if hdr2.Get("Idempotency-Replayed") != "true" {
		t.Error("replay must carry Idempotency-Replayed: true")
	}

	// Only ONE audit row — the second request short-circuited from
	// cache and did NOT re-enter the writeTx.
	if got := countAuditEvents(t, env, env.wsA, "contact.created"); got != 1 {
		t.Errorf("contact.created count after replay: got %d want 1", got)
	}
}

// --- Test 2: Idempotency-Key is scoped per workspace ---

func TestAuditIdempotency_KeyScopedPerWorkspace(t *testing.T) {
	env := setupPipelineEnv(t)
	key := "shared-key-" + uuid.New().String()

	stA, bodyA, _ := env.doJSONH(t, env.wsA, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": "A", "last_name": "One", "email": "a@a.test"},
		map[string]string{"Idempotency-Key": key},
	)
	if stA != http.StatusCreated {
		t.Fatalf("ws A POST: status=%d body=%s", stA, bodyA)
	}

	stB, bodyB, hdrB := env.doJSONH(t, env.wsB, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": "B", "last_name": "Two", "email": "b@b.test"},
		map[string]string{"Idempotency-Key": key},
	)
	if stB != http.StatusCreated {
		t.Fatalf("ws B POST: status=%d body=%s", stB, bodyB)
	}
	if hdrB.Get("Idempotency-Replayed") == "true" {
		t.Error("ws B must NOT replay ws A's cached response — cache is per-workspace")
	}
	if string(bodyA) == string(bodyB) {
		t.Errorf("ws A and ws B got identical responses; cache leaked across tenants")
	}
}

// --- Test 3: mutations emit the expected audit events ---

func TestAuditIdempotency_MutationsEmitAuditEvents(t *testing.T) {
	env := setupPipelineEnv(t)
	stages := env.listStages(t, env.wsA)

	st, body, _ := env.doJSONH(t, env.wsA, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": "X", "last_name": "Y", "email": "x@y.test"}, nil,
	)
	if st != http.StatusCreated {
		t.Fatalf("create: %d %s", st, body)
	}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode contact: %v", err)
	}

	st, body, _ = env.doJSONH(t, env.wsA, http.MethodPut, "/v1/contacts/"+created.ID.String(),
		map[string]any{"first_name": "X2", "last_name": "Y2", "email": "x2@y2.test"}, nil,
	)
	if st != http.StatusOK {
		t.Fatalf("update: %d %s", st, body)
	}

	st, body, _ = env.doJSONH(t, env.wsA, http.MethodDelete, "/v1/contacts/"+created.ID.String(), nil, nil)
	if st != http.StatusNoContent {
		t.Fatalf("delete: %d %s", st, body)
	}

	for _, ev := range []string{"contact.created", "contact.updated", "contact.deleted"} {
		if got := countAuditEvents(t, env, env.wsA, ev); got != 1 {
			t.Errorf("%s count: got %d want 1", ev, got)
		}
	}

	st, body, _ = env.doJSONH(t, env.wsA, http.MethodPost, "/v1/deals",
		map[string]any{"title": "Test Deal", "stage_id": stages[0].ID.String()}, nil,
	)
	if st != http.StatusCreated {
		t.Fatalf("create deal: %d %s", st, body)
	}
	if got := countAuditEvents(t, env, env.wsA, "deal.created"); got != 1 {
		t.Errorf("deal.created count: got %d want 1", got)
	}

	st, body, _ = env.doJSONH(t, env.wsA, http.MethodPost, "/v1/companies",
		map[string]any{"name": "Acme Co"}, nil,
	)
	if st != http.StatusCreated {
		t.Fatalf("create company: %d %s", st, body)
	}
	if got := countAuditEvents(t, env, env.wsA, "company.created"); got != 1 {
		t.Errorf("company.created count: got %d want 1", got)
	}
}

// --- Test 4: fail-closed — audit failure rolls back the mutation ---

func TestAuditIdempotency_FailClosedRollsBackMutation(t *testing.T) {
	env := setupPipelineEnv(t)

	// Drop core.audit_log to force every audit INSERT to fail. Same
	// trick metadata's fail_closed_test.go uses for its own fail-closed
	// proof — see apps/api/internal/metadata/fail_closed_test.go.
	if _, err := env.pool.Exec(context.Background(), "DROP TABLE core.audit_log CASCADE"); err != nil {
		t.Fatalf("drop audit_log: %v", err)
	}

	st, body, _ := env.doJSONH(t, env.wsA, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": "Should", "last_name": "Fail", "email": "should@fail.test"},
		nil,
	)
	if st != http.StatusInternalServerError {
		t.Fatalf("expected 500 (audit failure), got %d body=%s", st, body)
	}

	// Prove no contact row was persisted in the workspace schema.
	var n int
	q := `SELECT count(*) FROM "` + env.wsA.roleName + `".contacts WHERE first_name = 'Should'`
	if err := env.pool.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if n != 0 {
		t.Fatalf("fail-closed broken: contact persisted despite audit failure (count=%d)", n)
	}
}
