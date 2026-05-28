package crm

// Activities, Notes, and Tasks (ANT) REST handlers — Sprint 7
// features 4 + 5 (tasket 20260525-1004). Co-located in the crm package
// because they share the writeTx / pgtype helpers / audit wiring of the
// existing entity handlers.
//
// Activities surface is read-only: writes happen via emitActivity inside
// the same transaction as the originating entity mutation (fail-closed
// per ADR-009 §7.2).
//
// Notes and Tasks are full CRUD. Author / assignee identity comes from
// the request body today; once session middleware deposits a user
// identity into context (Sprint 9+, ADR-009 §7.2) the handlers will
// switch to deriving it from the session.

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gbconsult/lecrm/apps/api/internal/jobs"
	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// RegisterANTRoutes registers the Activities / Notes / Tasks routes.
// Separate from RegisterRoutes so the route table for the original
// entity handlers stays focused; both methods share the same Handler.
func (h *Handler) RegisterANTRoutes(r chi.Router) {
	// Activities (read-only)
	r.Get("/v1/contacts/{id}/activities", h.ListContactActivities)
	r.Get("/v1/companies/{id}/activities", h.ListCompanyActivities)
	r.Get("/v1/deals/{id}/activities", h.ListDealActivities)

	// Notes
	r.Get("/v1/contacts/{id}/notes", h.ListContactNotes)
	r.Get("/v1/companies/{id}/notes", h.ListCompanyNotes)
	r.Get("/v1/deals/{id}/notes", h.ListDealNotes)
	r.Post("/v1/contacts/{id}/notes", h.CreateContactNote)
	r.Post("/v1/companies/{id}/notes", h.CreateCompanyNote)
	r.Post("/v1/deals/{id}/notes", h.CreateDealNote)
	r.Put("/v1/notes/{id}", h.UpdateNote)
	r.Delete("/v1/notes/{id}", h.DeleteNote)

	// Tasks
	r.Get("/v1/tasks", h.ListTasks)
	r.Post("/v1/tasks", h.CreateTask)
	r.Put("/v1/tasks/{id}", h.UpdateTask)
	r.Patch("/v1/tasks/{id}/complete", h.ToggleTaskCompletion)
	r.Delete("/v1/tasks/{id}", h.DeleteTask)
}

// ========================================================
// Activities
// ========================================================

const activityPageLimit int32 = 100

type activityResp struct {
	ID           uuid.UUID       `json:"id"`
	EntityType   string          `json:"entity_type"`
	EntityID     uuid.UUID       `json:"entity_id"`
	ActorType    *string         `json:"actor_type"`
	ActorID      *string         `json:"actor_id"`
	EventType    string          `json:"event_type"`
	SourceSystem *string         `json:"source_system"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    time.Time       `json:"created_at"`
}

func activityFromRow(a sqlcgen.Activity) activityResp {
	payload := json.RawMessage(a.Payload)
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	return activityResp{
		ID:           a.ID,
		EntityType:   a.EntityType,
		EntityID:     a.EntityID,
		ActorType:    textPtr(a.ActorType),
		ActorID:      uuidPtr(a.ActorID),
		EventType:    a.EventType,
		SourceSystem: textPtr(a.SourceSystem),
		Payload:      payload,
		CreatedAt:    a.CreatedAt.Time,
	}
}

func (h *Handler) listActivities(w http.ResponseWriter, r *http.Request, entityType string) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var rows []sqlcgen.Activity
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListActivitiesByEntity(r.Context(), sqlcgen.ListActivitiesByEntityParams{
			EntityType: entityType,
			EntityID:   id,
			PageLimit:  activityPageLimit,
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list activities", "err", err, "entity_type", entityType)
		writeErr(w, http.StatusInternalServerError, "list activities failed")
		return
	}
	out := make([]activityResp, len(rows))
	for i, a := range rows {
		out[i] = activityFromRow(a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) ListContactActivities(w http.ResponseWriter, r *http.Request) {
	h.listActivities(w, r, entityTypeContact)
}
func (h *Handler) ListCompanyActivities(w http.ResponseWriter, r *http.Request) {
	h.listActivities(w, r, entityTypeCompany)
}
func (h *Handler) ListDealActivities(w http.ResponseWriter, r *http.Request) {
	h.listActivities(w, r, entityTypeDeal)
}

// ========================================================
// Notes
// ========================================================

const notePageLimit int32 = 100
const maxNoteBodyLen = 64 * 1024

type noteResp struct {
	ID         uuid.UUID `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   uuid.UUID `json:"entity_id"`
	Body       string    `json:"body"`
	AuthorID   uuid.UUID `json:"author_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func noteFromRow(n sqlcgen.Note) noteResp {
	return noteResp{
		ID: n.ID, EntityType: n.EntityType, EntityID: n.EntityID,
		Body: n.Body, AuthorID: n.AuthorID,
		CreatedAt: n.CreatedAt.Time, UpdatedAt: n.UpdatedAt.Time,
	}
}

func (h *Handler) listNotes(w http.ResponseWriter, r *http.Request, entityType string) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var rows []sqlcgen.Note
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListNotesByEntity(r.Context(), sqlcgen.ListNotesByEntityParams{
			EntityType: entityType,
			EntityID:   id,
			PageLimit:  notePageLimit,
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list notes", "err", err, "entity_type", entityType)
		writeErr(w, http.StatusInternalServerError, "list notes failed")
		return
	}
	out := make([]noteResp, len(rows))
	for i, n := range rows {
		out[i] = noteFromRow(n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) ListContactNotes(w http.ResponseWriter, r *http.Request) {
	h.listNotes(w, r, entityTypeContact)
}
func (h *Handler) ListCompanyNotes(w http.ResponseWriter, r *http.Request) {
	h.listNotes(w, r, entityTypeCompany)
}
func (h *Handler) ListDealNotes(w http.ResponseWriter, r *http.Request) {
	h.listNotes(w, r, entityTypeDeal)
}

type createNoteReq struct {
	Body     string `json:"body"`
	AuthorID string `json:"author_id"`
}

func (h *Handler) createNote(w http.ResponseWriter, r *http.Request, entityType string) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	entityID, ok := parseID(w, r)
	if !ok {
		return
	}
	var body createNoteReq
	if !decodeBody(w, r, &body) {
		return
	}
	body.Body = strings.TrimSpace(body.Body)
	if body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body must not be empty")
		return
	}
	if len(body.Body) > maxNoteBodyLen {
		writeErr(w, http.StatusBadRequest, "body exceeds 64 KiB")
		return
	}
	authorID, err := uuid.Parse(body.AuthorID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid author_id")
		return
	}

	var row sqlcgen.Note
	err = writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).CreateNote(r.Context(), sqlcgen.CreateNoteParams{
			EntityType: entityType,
			EntityID:   entityID,
			Body:       body.Body,
			AuthorID:   authorID,
		})
		if e != nil {
			return e
		}
		if e := emitAudit(r.Context(), tx, "note.created", ws.ID, map[string]any{
			"id": row.ID.String(), "entity_type": entityType, "entity_id": entityID.String(),
		}); e != nil {
			return e
		}
		return emitRESTActivity(r.Context(), tx, entityType, entityID, "note.added", map[string]any{
			"note_id": row.ID.String(), "author_id": authorID.String(),
		})
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "create note", "err", err)
		writeErr(w, http.StatusInternalServerError, "create note failed")
		return
	}
	writeJSON(w, http.StatusCreated, noteFromRow(row))
}

func (h *Handler) CreateContactNote(w http.ResponseWriter, r *http.Request) {
	h.createNote(w, r, entityTypeContact)
}
func (h *Handler) CreateCompanyNote(w http.ResponseWriter, r *http.Request) {
	h.createNote(w, r, entityTypeCompany)
}
func (h *Handler) CreateDealNote(w http.ResponseWriter, r *http.Request) {
	h.createNote(w, r, entityTypeDeal)
}

type updateNoteReq struct {
	Body     string `json:"body"`
	AuthorID string `json:"author_id"` // claimed author — must match existing row's author_id
}

func (h *Handler) UpdateNote(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	noteID, ok := parseID(w, r)
	if !ok {
		return
	}
	var body updateNoteReq
	if !decodeBody(w, r, &body) {
		return
	}
	body.Body = strings.TrimSpace(body.Body)
	if body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body must not be empty")
		return
	}
	if len(body.Body) > maxNoteBodyLen {
		writeErr(w, http.StatusBadRequest, "body exceeds 64 KiB")
		return
	}
	claimedAuthor, err := uuid.Parse(body.AuthorID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid author_id")
		return
	}

	var (
		row       sqlcgen.Note
		forbidden bool
		notFound  bool
	)
	err = writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		existing, e := q.GetNote(r.Context(), noteID)
		if errors.Is(e, pgx.ErrNoRows) {
			notFound = true
			return nil
		}
		if e != nil {
			return e
		}
		// Author check: only the note's author may edit. Admin override
		// is a Sprint 9+ concern (role-based middleware not yet wired).
		if existing.AuthorID != claimedAuthor {
			forbidden = true
			return nil
		}
		row, e = q.UpdateNote(r.Context(), sqlcgen.UpdateNoteParams{
			ID:   noteID,
			Body: body.Body,
		})
		if e != nil {
			return e
		}
		return emitAudit(r.Context(), tx, "note.updated", ws.ID, map[string]any{
			"id": noteID.String(),
		})
	})
	if notFound {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	if forbidden {
		writeErr(w, http.StatusForbidden, "only the author may edit this note")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "update note", "err", err)
		writeErr(w, http.StatusInternalServerError, "update note failed")
		return
	}
	writeJSON(w, http.StatusOK, noteFromRow(row))
}

// DeleteNote — author may always delete. The `author_id` query
// parameter identifies the caller. If empty, the request is treated
// as admin (Sprint 9+ will tie this to a role-based middleware check;
// for v0 this is a soft check used by the React admin UI).
func (h *Handler) DeleteNote(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	noteID, ok := parseID(w, r)
	if !ok {
		return
	}
	var claimedAuthor uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("author_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid author_id query parameter")
			return
		}
		claimedAuthor = parsed
	}

	var (
		forbidden bool
		notFound  bool
	)
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		existing, e := q.GetNote(r.Context(), noteID)
		if errors.Is(e, pgx.ErrNoRows) {
			notFound = true
			return nil
		}
		if e != nil {
			return e
		}
		// claimedAuthor == uuid.Nil → admin path (no author check).
		if claimedAuthor != uuid.Nil && existing.AuthorID != claimedAuthor {
			forbidden = true
			return nil
		}
		if e := q.DeleteNote(r.Context(), noteID); e != nil {
			return e
		}
		return emitAudit(r.Context(), tx, "note.deleted", ws.ID, map[string]any{
			"id": noteID.String(),
		})
	})
	if notFound {
		writeErr(w, http.StatusNotFound, "note not found")
		return
	}
	if forbidden {
		writeErr(w, http.StatusForbidden, "only the author may delete this note")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "delete note", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete note failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ========================================================
// Tasks
// ========================================================

const taskPageLimit int32 = 100

type taskResp struct {
	ID          uuid.UUID  `json:"id"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	EntityType  *string    `json:"entity_type"`
	EntityID    *string    `json:"entity_id"`
	AssigneeID  *string    `json:"assignee_id"`
	DueDate     *string    `json:"due_date"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func taskFromRow(t sqlcgen.Task) taskResp {
	return taskResp{
		ID:          t.ID,
		Title:       t.Title,
		Description: textPtr(t.Description),
		EntityType:  textPtr(t.EntityType),
		EntityID:    uuidPtr(t.EntityID),
		AssigneeID:  uuidPtr(t.AssigneeID),
		DueDate:     datePtr(t.DueDate),
		CompletedAt: tsPtr(t.CompletedAt),
		CreatedAt:   t.CreatedAt.Time,
		UpdatedAt:   t.UpdatedAt.Time,
	}
}

func validateEntityType(s string) bool {
	switch s {
	case entityTypeContact, entityTypeCompany, entityTypeDeal:
		return true
	}
	return false
}

func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	var assignee uuid.NullUUID
	if raw := strings.TrimSpace(q.Get("assignee_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid assignee_id")
			return
		}
		assignee = uuid.NullUUID{UUID: parsed, Valid: true}
	}
	entityType := strings.TrimSpace(q.Get("entity_type"))
	var entityID uuid.NullUUID
	if entityType != "" {
		if !validateEntityType(entityType) {
			writeErr(w, http.StatusBadRequest, "invalid entity_type")
			return
		}
		raw := strings.TrimSpace(q.Get("entity_id"))
		if raw == "" {
			writeErr(w, http.StatusBadRequest, "entity_id required when entity_type is set")
			return
		}
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid entity_id")
			return
		}
		entityID = uuid.NullUUID{UUID: parsed, Valid: true}
	}

	var rows []sqlcgen.Task
	err := readTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		rows, e = sqlcgen.New(tx).ListTasksByAssignee(r.Context(), sqlcgen.ListTasksByAssigneeParams{
			AssigneeID: assignee,
			EntityType: entityType,
			EntityID:   entityID,
			PageLimit:  taskPageLimit,
		})
		return e
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list tasks", "err", err)
		writeErr(w, http.StatusInternalServerError, "list tasks failed")
		return
	}
	out := make([]taskResp, len(rows))
	for i, t := range rows {
		out[i] = taskFromRow(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

type createTaskReq struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	EntityType  *string `json:"entity_type"`
	EntityID    *string `json:"entity_id"`
	AssigneeID  *string `json:"assignee_id"`
	DueDate     *string `json:"due_date"`
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	var body createTaskReq
	if !decodeBody(w, r, &body) {
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		writeErr(w, http.StatusBadRequest, "title must not be empty")
		return
	}
	entityType := pgtype.Text{}
	entityID := uuid.NullUUID{}
	if body.EntityType != nil && *body.EntityType != "" {
		if !validateEntityType(*body.EntityType) {
			writeErr(w, http.StatusBadRequest, "invalid entity_type")
			return
		}
		if body.EntityID == nil || *body.EntityID == "" {
			writeErr(w, http.StatusBadRequest, "entity_id required when entity_type is set")
			return
		}
		parsed, err := uuid.Parse(*body.EntityID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid entity_id")
			return
		}
		entityType = pgtype.Text{String: *body.EntityType, Valid: true}
		entityID = uuid.NullUUID{UUID: parsed, Valid: true}
	}

	dueDate := toDate(body.DueDate)
	if body.DueDate != nil && *body.DueDate != "" && !dueDate.Valid {
		writeErr(w, http.StatusBadRequest, "invalid due_date (expect YYYY-MM-DD)")
		return
	}

	var row sqlcgen.Task
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).CreateTask(r.Context(), sqlcgen.CreateTaskParams{
			Title:       body.Title,
			Description: toText(body.Description),
			EntityType:  entityType,
			EntityID:    entityID,
			AssigneeID:  toNullUUID(body.AssigneeID),
			DueDate:     dueDate,
		})
		if e != nil {
			return e
		}
		return emitAudit(r.Context(), tx, "task.created", ws.ID, map[string]any{
			"id": row.ID.String(), "title": row.Title,
		})
	})
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "create task", "err", err)
		writeErr(w, http.StatusInternalServerError, "create task failed")
		return
	}
	h.maybeEnqueueReminder(r, ws.ID, row)
	writeJSON(w, http.StatusCreated, taskFromRow(row))
}

type updateTaskReq createTaskReq

func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	taskID, ok := parseID(w, r)
	if !ok {
		return
	}
	var body updateTaskReq
	if !decodeBody(w, r, &body) {
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	if body.Title == "" {
		writeErr(w, http.StatusBadRequest, "title must not be empty")
		return
	}
	entityType := pgtype.Text{}
	entityID := uuid.NullUUID{}
	if body.EntityType != nil && *body.EntityType != "" {
		if !validateEntityType(*body.EntityType) {
			writeErr(w, http.StatusBadRequest, "invalid entity_type")
			return
		}
		if body.EntityID == nil || *body.EntityID == "" {
			writeErr(w, http.StatusBadRequest, "entity_id required when entity_type is set")
			return
		}
		parsed, err := uuid.Parse(*body.EntityID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid entity_id")
			return
		}
		entityType = pgtype.Text{String: *body.EntityType, Valid: true}
		entityID = uuid.NullUUID{UUID: parsed, Valid: true}
	}
	dueDate := toDate(body.DueDate)
	if body.DueDate != nil && *body.DueDate != "" && !dueDate.Valid {
		writeErr(w, http.StatusBadRequest, "invalid due_date (expect YYYY-MM-DD)")
		return
	}

	var row sqlcgen.Task
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).UpdateTask(r.Context(), sqlcgen.UpdateTaskParams{
			ID:          taskID,
			Title:       body.Title,
			Description: toText(body.Description),
			EntityType:  entityType,
			EntityID:    entityID,
			AssigneeID:  toNullUUID(body.AssigneeID),
			DueDate:     dueDate,
		})
		if e != nil {
			return e
		}
		return emitAudit(r.Context(), tx, "task.updated", ws.ID, map[string]any{
			"id": row.ID.String(),
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "update task", "err", err)
		writeErr(w, http.StatusInternalServerError, "update task failed")
		return
	}
	h.maybeEnqueueReminder(r, ws.ID, row)
	writeJSON(w, http.StatusOK, taskFromRow(row))
}

func (h *Handler) ToggleTaskCompletion(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	taskID, ok := parseID(w, r)
	if !ok {
		return
	}
	var row sqlcgen.Task
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		var e error
		row, e = sqlcgen.New(tx).ToggleTaskCompletion(r.Context(), taskID)
		if e != nil {
			return e
		}
		event := "task.completed"
		if !row.CompletedAt.Valid {
			event = "task.reopened"
		}
		return emitAudit(r.Context(), tx, event, ws.ID, map[string]any{
			"id": row.ID.String(),
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "toggle task", "err", err)
		writeErr(w, http.StatusInternalServerError, "toggle task failed")
		return
	}
	writeJSON(w, http.StatusOK, taskFromRow(row))
}

func (h *Handler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.ws(w, r)
	if !ok {
		return
	}
	taskID, ok := parseID(w, r)
	if !ok {
		return
	}
	err := writeTx(r.Context(), h.Pool, ws.RoleName, func(tx pgx.Tx) error {
		if e := deleteRow(r.Context(), tx, "tasks", taskID); e != nil {
			return e
		}
		return emitAudit(r.Context(), tx, "task.deleted", ws.ID, map[string]any{
			"id": taskID.String(),
		})
	})
	if errors.Is(err, errNotFound) {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "delete task", "err", err)
		writeErr(w, http.StatusInternalServerError, "delete task failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maybeEnqueueReminder fires a TaskReminder job through the optional
// runner when the task carries a future due_date and is not yet
// completed. The runner is a placeholder until River is added — for
// v0 it just logs the enqueue event (see RiverAdapter.Enqueue). Errors
// are logged but never block the HTTP response: a missed reminder is
// a usability bug, not a data-integrity bug.
func (h *Handler) maybeEnqueueReminder(r *http.Request, workspaceID uuid.UUID, t sqlcgen.Task) {
	if h.JobRunner == nil {
		return
	}
	if !t.DueDate.Valid || t.CompletedAt.Valid {
		return
	}
	job := jobs.NewTaskReminderJob(workspaceID, t.ID, t.DueDate.Time)
	if err := h.JobRunner.Enqueue(r.Context(), job); err != nil {
		h.Logger.WarnContext(r.Context(), "enqueue task reminder failed",
			"task_id", t.ID.String(), "err", err)
	}
}
