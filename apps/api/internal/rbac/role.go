// Package rbac implements the workspace-scoped, role-based authorization
// layer for leCRM v0 (Feature 7, Sprint 8).
//
// The role hierarchy is a total order — member < admin < owner — recorded
// per workspace in core.workspace_members.role. A request's effective role
// is resolved once by Resolve (which deposits a *Principal into the request
// context) and then enforced by RequireRole / RequireRoleByMethod.
//
// Capability summary (see Permissions):
//
//   - member:     read all CRM entities.
//   - admin:      member + create / update / delete CRM entities.
//   - owner:      admin + manage workspace members + service tokens +
//     delete the workspace.
//   - integrator: GB Consult's cross-workspace integrator principal. Sits at
//     the top of the total order with owner-equivalent capabilities, but is a
//     distinct, non-billable identity (hidden from the client member list,
//     actions tagged in core.audit_log). Materialized at login time from a
//     core.integrator_grants pending grant.
package rbac

import "strings"

// Role is a workspace membership level. The zero value (RoleNone) means
// "no membership" — an unauthenticated or non-member principal.
type Role int

const (
	// RoleNone is the absence of a workspace role. It satisfies no
	// RequireRole gate (RequireRole treats it as unauthenticated).
	RoleNone Role = iota
	// RoleMember can read all entities.
	RoleMember
	// RoleAdmin can read and mutate entities.
	RoleAdmin
	// RoleOwner can do everything admin can, plus manage members,
	// service tokens, and delete the workspace.
	RoleOwner
	// RoleIntegrator is GB Consult's cross-workspace integrator principal.
	// It is the highest level in the total order and yields owner-equivalent
	// capabilities, but is a distinct, non-billable identity (hidden from the
	// client member list; actions tagged in core.audit_log). It is
	// materialized at login from a core.integrator_grants pending grant.
	RoleIntegrator
)

// roleNames maps each role to its canonical wire string. RoleNone has no
// wire representation (it never round-trips through the database).
var roleNames = map[Role]string{
	RoleMember:     "member",
	RoleAdmin:      "admin",
	RoleOwner:      "owner",
	RoleIntegrator: "integrator",
}

// String returns the canonical wire name ("member"/"admin"/"owner"), or
// "none" for RoleNone / unknown values.
func (r Role) String() string {
	if s, ok := roleNames[r]; ok {
		return s
	}
	return "none"
}

// ParseRole maps a database/wire role string to a Role. The second return
// is false for unknown values (callers MUST treat that as RoleNone and
// deny, never as a silent member grant).
func ParseRole(s string) (Role, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "member":
		return RoleMember, true
	case "admin":
		return RoleAdmin, true
	case "owner":
		return RoleOwner, true
	case "integrator":
		return RoleIntegrator, true
	default:
		return RoleNone, false
	}
}

// AtLeast reports whether r is at or above min in the hierarchy. RoleNone
// is below every real role, so it never satisfies a gate.
func (r Role) AtLeast(min Role) bool { return r >= min && r != RoleNone }

// Permissions is the capability bundle for a role. It is the shape returned
// by GET /v1/workspace/me so the frontend can gate controls without
// re-deriving the hierarchy.
type Permissions struct {
	CanRead            bool `json:"can_read"`
	CanWrite           bool `json:"can_write"`
	CanManageMembers   bool `json:"can_manage_members"`
	CanManageTokens    bool `json:"can_manage_tokens"`
	CanDeleteWorkspace bool `json:"can_delete_workspace"`
}

// PermissionsFor expands a role into its capability bundle. Because the role
// hierarchy is a total order and RoleIntegrator sits above RoleOwner, the
// AtLeast(RoleOwner) checks below already grant the integrator every
// owner-equivalent capability.
func PermissionsFor(r Role) Permissions {
	return Permissions{
		CanRead:            r.AtLeast(RoleMember),
		CanWrite:           r.AtLeast(RoleAdmin),
		CanManageMembers:   r.AtLeast(RoleOwner),
		CanManageTokens:    r.AtLeast(RoleOwner),
		CanDeleteWorkspace: r.AtLeast(RoleOwner),
	}
}

// roleFromScopes maps a service-token's scope set to an effective role.
//
// Service tokens are deliberately capped at RoleAdmin: member management,
// token administration, and workspace deletion are owner-only and must be
// performed by a human session, never delegated to a long-lived token.
// A token bearing the wildcard "*" scope therefore resolves to admin
// (full CRM read/write), NOT owner.
//
//   - any "*" or "*:write" / "<entity>:write" / "write" scope → admin
//   - read-only scopes ("read", "<entity>:read")             → member
//   - empty / unrecognized                                   → member
func roleFromScopes(scopes []string) Role {
	role := RoleMember
	for _, s := range scopes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "*" || strings.Contains(s, "write") {
			return RoleAdmin
		}
	}
	return role
}
