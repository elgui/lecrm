package auth

// BearerAuthenticator is the workspace.BearerAuthenticator
// implementation: it pulls active candidate token rows from
// core.service_tokens via a pluggable CandidateLoader and verifies
// the inbound bearer against each row.
//
// The package-level adapter satisfies the workspace.BearerAuthenticator
// interface without taking a hard dependency on the workspace package
// (workspace only imports auth via the interface, not the type).

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// HTTPBearerAuthenticator is the production implementation wired into
// workspace.MiddlewareWithBearer. The zero value is NOT usable: the
// Loader field MUST be set.
type HTTPBearerAuthenticator struct {
	Loader CandidateLoader
}

// Authenticate satisfies the workspace.BearerAuthenticator interface.
// When the Authorization header is missing this method is not invoked
// (the workspace middleware guards on header presence). When the
// bearer is malformed or no candidate matches, an error is returned
// and the middleware writes 401.
func (a *HTTPBearerAuthenticator) Authenticate(r *http.Request, workspaceID uuid.UUID, workspaceSlug string) (context.Context, error) {
	plaintext := ExtractBearer(r)
	if plaintext == "" {
		// Malformed header (e.g. "Authorization: Basic …"). Treat as
		// "no bearer present" — caller will then 401 only if no
		// session cookie is present.
		return nil, nil
	}
	actor, err := VerifyBearer(r.Context(), a.Loader, workspaceID, workspaceSlug, plaintext)
	if err != nil {
		return nil, err
	}
	return WithBearerActor(r.Context(), actor), nil
}
