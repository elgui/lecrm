// Package workspace carries the per-request workspace identity through
// the request context. The workspace is resolved once at the edge from
// the request subdomain (see Middleware) and then read by downstream
// handlers via WorkspaceFromContext.
//
// The context key is an unexported type so callers cannot construct or
// shadow it from outside this package — the standard Go idiom that
// avoids the "string key" pitfall flagged by golangci-lint's `staticcheck`.
package workspace

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Context is the typed value attached to *http.Request contexts by
// Middleware. RoleName is the per-workspace Postgres role from
// ADR-009 §2.1 — the connection-string-builder uses it to open the
// per-tenant data connection.
type Context struct {
	ID       uuid.UUID
	Slug     string
	RoleName string
}

type ctxKey struct{}

// ErrMissingWorkspace is returned by WorkspaceFromContext when no
// workspace was attached. A handler that requires a workspace MUST
// treat this as a 500 (the middleware failed to run) rather than a
// 401/404 — the latter would leak information about the routing layer.
var ErrMissingWorkspace = errors.New("workspace: no workspace in context (middleware not wired?)")

// WithWorkspace returns a derived context carrying ws.
func WithWorkspace(ctx context.Context, ws *Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, ws)
}

// WorkspaceFromContext extracts the workspace previously attached by
// Middleware. Returns ErrMissingWorkspace when no workspace is present.
func WorkspaceFromContext(ctx context.Context) (*Context, error) {
	ws, ok := ctx.Value(ctxKey{}).(*Context)
	if !ok || ws == nil {
		return nil, ErrMissingWorkspace
	}
	return ws, nil
}
