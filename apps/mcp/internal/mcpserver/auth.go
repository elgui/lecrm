package mcpserver

import (
	"errors"
	"net/http"
	"strings"

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

// scopesHeader carries the token's granted scopes, stamped by the edge gateway
// after it verifies the service token (the same lecrm_ service-token table the
// API uses, ADR-009 §4.1). The capability layer maps these scopes to a write
// role; a read-only scope set is rejected before any mutation (ADR-012 §6/§8).
// Mechanism-agnostic (ADR-012 §7): a future OAuth access token's scopes arrive
// through the same header with zero tool changes.
const scopesHeader = "X-Lecrm-Scopes"

// scopesFromRequest parses the granted scopes from the request. Scopes are
// space- or comma-separated; blanks are dropped. A read-only deployment that
// never sets the header yields nil, which RoleFromScopes treats as read-only.
func scopesFromRequest(r *http.Request) []string {
	raw := r.Header.Get(scopesHeader)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(c rune) bool { return c == ',' || c == ' ' || c == '\t' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
