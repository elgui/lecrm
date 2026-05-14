package workspace

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gbconsult/lecrm/apps/api/internal/sqlcgen"
)

// TestListHandler is the ADR-009 §1.1 Test 1 surface: a `/v1/_test/workspaces`
// GET handler that pulls rows through an sqlc-generated method and
// returns them as JSON. It REQUIRES a workspace in context — the
// handler is wrapped by Middleware in the router and refuses to run
// otherwise.
//
// The handler is named `TestList…` (not `_test`-suffixed) because the
// `_test` package convention is reserved for Go test files; the route
// path is `/v1/_test/workspaces` but the type is not.
type TestListHandler struct {
	Pool   *pgxpool.Pool
	Logger *slog.Logger
}

type listResponseItem struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type listResponse struct {
	Workspace listResponseItem   `json:"workspace"`
	Items     []listResponseItem `json:"items"`
}

func (h *TestListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := WorkspaceFromContext(r.Context())
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "workspace missing in test handler", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "workspace context missing")
		return
	}

	q := sqlcgen.New(h.Pool)
	rows, err := q.ListWorkspacesForTest(r.Context())
	if err != nil {
		h.Logger.ErrorContext(r.Context(), "list workspaces failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "list failed")
		return
	}

	items := make([]listResponseItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, listResponseItem{
			ID:        row.ID.String(),
			Slug:      row.Slug,
			CreatedAt: row.CreatedAt.Time,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(listResponse{
		Workspace: listResponseItem{ID: ws.ID.String(), Slug: ws.Slug},
		Items:     items,
	}); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		h.Logger.ErrorContext(r.Context(), "encode failed", "err", err)
	}
}
