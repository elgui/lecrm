package mcpserver

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
)

// workspaceHeader carries the resolved workspace UUID. In production the
// edge gateway verifies the service token (shared lecrm_ service-token
// table, ADR-009 §4.1) and injects this header; the MCP service trusts
// it and scopes the constrained RO role to that workspace. Keeping token
// verification at the gateway avoids duplicating the argon2id verifier
// in this binary for the v0 skeleton.
const workspaceHeader = "X-Lecrm-Workspace-Id"

// workspaceFromRequest extracts and validates the workspace UUID the
// request is scoped to.
func workspaceFromRequest(r *http.Request) (uuid.UUID, error) {
	raw := r.Header.Get(workspaceHeader)
	if raw == "" {
		return uuid.Nil, errors.New("missing " + workspaceHeader + " header")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.New("invalid workspace id")
	}
	if id == uuid.Nil {
		return uuid.Nil, errors.New("workspace id must not be nil")
	}
	return id, nil
}
