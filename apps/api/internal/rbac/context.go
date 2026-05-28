package rbac

import (
	"context"

	"github.com/google/uuid"
)

// Principal is the resolved authorization identity for a request. It is
// deposited into the request context by Resolve and read by RequireRole,
// RequireRoleByMethod, and the /v1/workspace/me handler.
//
// Exactly one of the two authentication paths populates it:
//
//   - session cookie  → UserID set, ActorType "human_api", IsServiceToken false
//   - bearer token    → UserID zero, ActorType from the token, IsServiceToken true
type Principal struct {
	Role           Role
	UserID         uuid.UUID
	ActorType      string
	IsServiceToken bool
}

type principalCtxKey struct{}

// WithPrincipal returns a derived context carrying p.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFromContext returns the principal deposited by Resolve, or
// (nil, false) when the request was not resolved to a workspace member or
// authorized service token.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(*Principal)
	return p, ok && p != nil
}
