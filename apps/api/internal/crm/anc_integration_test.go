//go:build integration

// Integration tests for Activities + Notes + Tasks (Sprint 7 features
// 4+5, tasket 20260525-1004). Shares the pipelineTestEnv harness from
// pipeline_integration_test.go.
//
// Run:
//
//	~/.local/go/bin/go -C apps/api test -tags integration -count 1 -race -v \
//	    -run "TestANT_" ./internal/crm

package crm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type antContact struct {
	ID uuid.UUID `json:"id"`
}

type antActivity struct {
	ID         uuid.UUID       `json:"id"`
	EntityType string          `json:"entity_type"`
	EntityID   uuid.UUID       `json:"entity_id"`
	EventType  string          `json:"event_type"`
	ActorType  *string         `json:"actor_type"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

type antNote struct {
	ID       uuid.UUID `json:"id"`
	Body     string    `json:"body"`
	AuthorID uuid.UUID `json:"author_id"`
}

type antTask struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	CompletedAt *time.Time `json:"completed_at"`
	DueDate     *string    `json:"due_date"`
}

func (e *pipelineTestEnv) createContact(t *testing.T, ws workspaceFixture, first, last, email string) antContact {
	t.Helper()
	status, body := e.doJSON(t, ws, http.MethodPost, "/v1/contacts",
		map[string]any{"first_name": first, "last_name": last, "email": email})
	if status != http.StatusCreated {
		t.Fatalf("create contact: %d %s", status, body)
	}
	var c antContact
	if err := json.Unmarshal(body, &c); err != nil {
		t.Fatalf("decode contact: %v body=%s", err, body)
	}
	return c
}

// --- TestANT_ActivityTimeline_OrderAndContent ---

func TestANT_ActivityTimeline_ReturnsEventsInReverseChronologicalOrder(t *testing.T) {
	env := setupPipelineEnv(t)
	c := env.createContact(t, env.wsA, "Alan", "Turing", "alan@enigma.test")

	// Force two more activity events: an update and a note add.
	st, body := env.doJSON(t, env.wsA, http.MethodPut, "/v1/contacts/"+c.ID.String(),
		map[string]any{"first_name": "Alan M.", "last_name": "Turing", "email": "alan.m@enigma.test"})
	if st != http.StatusOK {
		t.Fatalf("update contact: %d %s", st, body)
	}

	author := uuid.New()
	st, body = env.doJSON(t, env.wsA, http.MethodPost, "/v1/contacts/"+c.ID.String()+"/notes",
		map[string]any{"body": "first follow-up call scheduled", "author_id": author.String()})
	if st != http.StatusCreated {
		t.Fatalf("create note: %d %s", st, body)
	}

	st, body = env.doJSON(t, env.wsA, http.MethodGet,
		"/v1/contacts/"+c.ID.String()+"/activities", nil)
	if st != http.StatusOK {
		t.Fatalf("list activities: %d %s", st, body)
	}
	var resp struct {
		Data []antActivity `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode activities: %v body=%s", err, body)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("activities count: got %d want 3 (created/updated/note.added); body=%s", len(resp.Data), body)
	}
	// Reverse chronological: note.added → entity.updated → entity.created.
	wantOrder := []string{"note.added", "entity.updated", "entity.created"}
	for i, a := range resp.Data {
		if a.EventType != wantOrder[i] {
			t.Errorf("activities[%d].event_type: got %q want %q", i, a.EventType, wantOrder[i])
		}
		if a.ActorType == nil || *a.ActorType != "human_api" {
			t.Errorf("activities[%d].actor_type: got %v want human_api", i, a.ActorType)
		}
	}
}

// --- TestANT_ActivityCreatedAutomaticallyOnEntityMutation ---

func TestANT_ActivityCreatedAutomaticallyOnDealMutations(t *testing.T) {
	env := setupPipelineEnv(t)
	stages := env.listStages(t, env.wsA)
	deal := env.createDeal(t, env.wsA, "Pipeline Deal", stages[0].ID)

	// Update deal title.
	st, _ := env.doJSON(t, env.wsA, http.MethodPut, "/v1/deals/"+deal.ID.String(),
		map[string]any{"title": "Pipeline Deal Renamed", "stage_id": stages[0].ID.String()})
	if st != http.StatusOK {
		t.Fatalf("update deal: %d", st)
	}
	// Stage change.
	st, _ = env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/deals/"+deal.ID.String()+"/stage",
		map[string]any{"stage_id": stages[1].ID.String()})
	if st != http.StatusOK {
		t.Fatalf("stage transition: %d", st)
	}

	st, body := env.doJSON(t, env.wsA, http.MethodGet,
		"/v1/deals/"+deal.ID.String()+"/activities", nil)
	if st != http.StatusOK {
		t.Fatalf("list deal activities: %d %s", st, body)
	}
	var resp struct {
		Data []antActivity `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	want := []string{"deal.stage_changed", "entity.updated", "entity.created"}
	if len(resp.Data) != len(want) {
		t.Fatalf("expected %d activities, got %d; body=%s", len(want), len(resp.Data), body)
	}
	for i, ev := range want {
		if resp.Data[i].EventType != ev {
			t.Errorf("activity[%d].event_type: got %q want %q", i, resp.Data[i].EventType, ev)
		}
	}
}

// --- TestANT_NotesCRUDLifecycle ---

func TestANT_NotesCRUDLifecycle(t *testing.T) {
	env := setupPipelineEnv(t)
	c := env.createContact(t, env.wsA, "Grace", "Hopper", "grace@navy.test")
	author := uuid.New()
	other := uuid.New()

	// CREATE
	st, body := env.doJSON(t, env.wsA, http.MethodPost, "/v1/contacts/"+c.ID.String()+"/notes",
		map[string]any{"body": "first contact", "author_id": author.String()})
	if st != http.StatusCreated {
		t.Fatalf("create: %d %s", st, body)
	}
	var n antNote
	_ = json.Unmarshal(body, &n)
	if n.Body != "first contact" || n.AuthorID != author {
		t.Fatalf("note shape unexpected: %+v", n)
	}

	// LIST
	st, body = env.doJSON(t, env.wsA, http.MethodGet, "/v1/contacts/"+c.ID.String()+"/notes", nil)
	if st != http.StatusOK {
		t.Fatalf("list notes: %d %s", st, body)
	}
	var listResp struct {
		Data []antNote `json:"data"`
	}
	_ = json.Unmarshal(body, &listResp)
	if len(listResp.Data) != 1 || listResp.Data[0].ID != n.ID {
		t.Fatalf("list mismatch: %+v", listResp.Data)
	}

	// UPDATE — wrong author → 403
	st, _ = env.doJSON(t, env.wsA, http.MethodPut, "/v1/notes/"+n.ID.String(),
		map[string]any{"body": "edited", "author_id": other.String()})
	if st != http.StatusForbidden {
		t.Fatalf("non-author edit: got %d want 403", st)
	}

	// UPDATE — author → 200
	st, body = env.doJSON(t, env.wsA, http.MethodPut, "/v1/notes/"+n.ID.String(),
		map[string]any{"body": "edited by author", "author_id": author.String()})
	if st != http.StatusOK {
		t.Fatalf("author edit: %d %s", st, body)
	}
	var edited antNote
	_ = json.Unmarshal(body, &edited)
	if edited.Body != "edited by author" {
		t.Fatalf("body not updated: %q", edited.Body)
	}

	// DELETE — wrong author → 403
	st, _ = env.doJSON(t, env.wsA, http.MethodDelete,
		"/v1/notes/"+n.ID.String()+"?author_id="+other.String(), nil)
	if st != http.StatusForbidden {
		t.Fatalf("non-author delete: %d want 403", st)
	}

	// DELETE — author → 204
	st, _ = env.doJSON(t, env.wsA, http.MethodDelete,
		"/v1/notes/"+n.ID.String()+"?author_id="+author.String(), nil)
	if st != http.StatusNoContent {
		t.Fatalf("author delete: %d want 204", st)
	}

	// DELETE again → 404
	st, _ = env.doJSON(t, env.wsA, http.MethodDelete,
		"/v1/notes/"+n.ID.String()+"?author_id="+author.String(), nil)
	if st != http.StatusNotFound {
		t.Fatalf("re-delete: %d want 404", st)
	}
}

// --- TestANT_NoteCreationEmitsActivity ---

func TestANT_NoteCreationEmitsNoteAddedActivity(t *testing.T) {
	env := setupPipelineEnv(t)
	c := env.createContact(t, env.wsA, "Edsger", "Dijkstra", "edsger@goto.test")
	author := uuid.New()

	st, _ := env.doJSON(t, env.wsA, http.MethodPost, "/v1/contacts/"+c.ID.String()+"/notes",
		map[string]any{"body": "test note", "author_id": author.String()})
	if st != http.StatusCreated {
		t.Fatalf("create note: %d", st)
	}

	// Activities should now include "note.added" (and "entity.created").
	q := fmt.Sprintf(
		`SELECT count(*) FROM %q.activities WHERE entity_type='contact' AND entity_id=$1 AND event_type='note.added'`,
		env.wsA.roleName,
	)
	var n int
	if err := env.pool.QueryRow(context.Background(), q, c.ID).Scan(&n); err != nil {
		t.Fatalf("count activities: %v", err)
	}
	if n != 1 {
		t.Fatalf("note.added activity count: got %d want 1", n)
	}
}

// --- TestANT_TasksCRUDAndCompletion ---

func TestANT_TasksCRUDAndCompletionToggle(t *testing.T) {
	env := setupPipelineEnv(t)
	assignee := uuid.New()
	due := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02")

	// CREATE
	st, body := env.doJSON(t, env.wsA, http.MethodPost, "/v1/tasks",
		map[string]any{
			"title":       "Follow up with prospect",
			"assignee_id": assignee.String(),
			"due_date":    due,
		})
	if st != http.StatusCreated {
		t.Fatalf("create task: %d %s", st, body)
	}
	var task antTask
	_ = json.Unmarshal(body, &task)
	if task.CompletedAt != nil {
		t.Fatalf("new task should not be completed: %+v", task)
	}
	if task.DueDate == nil || *task.DueDate != due {
		t.Fatalf("due_date mismatch: got %v want %s", task.DueDate, due)
	}

	// LIST by assignee
	st, body = env.doJSON(t, env.wsA, http.MethodGet,
		"/v1/tasks?assignee_id="+assignee.String(), nil)
	if st != http.StatusOK {
		t.Fatalf("list tasks: %d %s", st, body)
	}
	var listResp struct {
		Data []antTask `json:"data"`
	}
	_ = json.Unmarshal(body, &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("expected 1 task for assignee, got %d", len(listResp.Data))
	}

	// COMPLETE
	st, body = env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/tasks/"+task.ID.String()+"/complete", nil)
	if st != http.StatusOK {
		t.Fatalf("toggle complete: %d %s", st, body)
	}
	var completed antTask
	_ = json.Unmarshal(body, &completed)
	if completed.CompletedAt == nil {
		t.Fatalf("expected completed_at to be set, got nil")
	}

	// TOGGLE back to open
	st, body = env.doJSON(t, env.wsA, http.MethodPatch,
		"/v1/tasks/"+task.ID.String()+"/complete", nil)
	if st != http.StatusOK {
		t.Fatalf("re-toggle: %d %s", st, body)
	}
	var reopened antTask
	_ = json.Unmarshal(body, &reopened)
	if reopened.CompletedAt != nil {
		t.Fatalf("expected completed_at nil after re-toggle, got %v", reopened.CompletedAt)
	}

	// UPDATE
	st, body = env.doJSON(t, env.wsA, http.MethodPut,
		"/v1/tasks/"+task.ID.String(),
		map[string]any{
			"title":       "Updated title",
			"assignee_id": assignee.String(),
			"due_date":    due,
		})
	if st != http.StatusOK {
		t.Fatalf("update task: %d %s", st, body)
	}
	var updated antTask
	_ = json.Unmarshal(body, &updated)
	if updated.Title != "Updated title" {
		t.Fatalf("title not updated: %q", updated.Title)
	}

	// DELETE
	st, _ = env.doJSON(t, env.wsA, http.MethodDelete, "/v1/tasks/"+task.ID.String(), nil)
	if st != http.StatusNoContent {
		t.Fatalf("delete: %d want 204", st)
	}
	st, _ = env.doJSON(t, env.wsA, http.MethodDelete, "/v1/tasks/"+task.ID.String(), nil)
	if st != http.StatusNotFound {
		t.Fatalf("re-delete: %d want 404", st)
	}
}

// --- TestANT_CrossTenantIsolation ---

func TestANT_CrossTenantIsolation_NotesAndTasksScopedToWorkspace(t *testing.T) {
	env := setupPipelineEnv(t)
	contactA := env.createContact(t, env.wsA, "Tenant", "A", "a@tenant.test")
	contactB := env.createContact(t, env.wsB, "Tenant", "B", "b@tenant.test")
	authorA := uuid.New()
	authorB := uuid.New()

	// Each workspace creates a note on its own contact.
	st, body := env.doJSON(t, env.wsA, http.MethodPost,
		"/v1/contacts/"+contactA.ID.String()+"/notes",
		map[string]any{"body": "secret-A", "author_id": authorA.String()})
	if st != http.StatusCreated {
		t.Fatalf("ws A create note: %d %s", st, body)
	}
	var noteA antNote
	_ = json.Unmarshal(body, &noteA)

	st, body = env.doJSON(t, env.wsB, http.MethodPost,
		"/v1/contacts/"+contactB.ID.String()+"/notes",
		map[string]any{"body": "secret-B", "author_id": authorB.String()})
	if st != http.StatusCreated {
		t.Fatalf("ws B create note: %d %s", st, body)
	}

	// Workspace B cannot see workspace A's note even with the right ID.
	st, body = env.doJSON(t, env.wsB, http.MethodPut, "/v1/notes/"+noteA.ID.String(),
		map[string]any{"body": "tampered", "author_id": authorA.String()})
	if st != http.StatusNotFound {
		t.Fatalf("cross-tenant note edit: %d want 404 body=%s", st, body)
	}

	// Workspace B's task list does NOT include workspace A's tasks.
	dueA := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	st, body = env.doJSON(t, env.wsA, http.MethodPost, "/v1/tasks",
		map[string]any{"title": "task-A", "assignee_id": authorA.String(), "due_date": dueA})
	if st != http.StatusCreated {
		t.Fatalf("ws A create task: %d %s", st, body)
	}

	st, body = env.doJSON(t, env.wsB, http.MethodGet,
		"/v1/tasks?assignee_id="+authorA.String(), nil)
	if st != http.StatusOK {
		t.Fatalf("ws B list tasks: %d %s", st, body)
	}
	var listResp struct {
		Data []antTask `json:"data"`
	}
	_ = json.Unmarshal(body, &listResp)
	if len(listResp.Data) != 0 {
		t.Fatalf("ws B saw ws A's tasks: %+v", listResp.Data)
	}

	// Workspace B's activity list for contact A returns empty
	// (the contact does not exist in B's schema, so no activities).
	st, body = env.doJSON(t, env.wsB, http.MethodGet,
		"/v1/contacts/"+contactA.ID.String()+"/activities", nil)
	if st != http.StatusOK {
		t.Fatalf("ws B list activities for A's contact: %d %s", st, body)
	}
	var actResp struct {
		Data []antActivity `json:"data"`
	}
	_ = json.Unmarshal(body, &actResp)
	if len(actResp.Data) != 0 {
		t.Fatalf("ws B saw activities for ws A's contact: %+v", actResp.Data)
	}
}
