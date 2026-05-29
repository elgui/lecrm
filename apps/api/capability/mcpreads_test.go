package capability

import (
	"testing"

	"github.com/google/uuid"
)

func TestMCPReadRole_MatchesProvisioningConvention(t *testing.T) {
	// Must match core.lecrm_provision_workspace:
	//   'workspace_' || lower(replace(uuid::text,'-','')) || '_ro'
	id := uuid.MustParse("0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9")
	got := MCPReadRole(id)
	want := "workspace_0a1b2c3d4e5f60718293a4b5c6d7e8f9_ro"
	if got != want {
		t.Fatalf("MCPReadRole = %q, want %q", got, want)
	}
}

func TestMCPReadRole_OnlySafeChars(t *testing.T) {
	got := MCPReadRole(uuid.New())
	for _, c := range got {
		ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || c == '_'
		if !ok {
			t.Fatalf("MCPReadRole produced unsafe char %q in %q", c, got)
		}
	}
}

func TestMCPReadPrincipal_PinsRoleAndSchema(t *testing.T) {
	ws := uuid.MustParse("0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9")
	p := MCPReadPrincipal(ws)
	if p.Schema != "workspace_0a1b2c3d4e5f60718293a4b5c6d7e8f9" {
		t.Fatalf("Schema = %q", p.Schema)
	}
	if p.ReadRole != "workspace_0a1b2c3d4e5f60718293a4b5c6d7e8f9_ro" {
		t.Fatalf("ReadRole = %q", p.ReadRole)
	}
	if p.Role != RoleMember {
		t.Fatalf("Role = %v, want RoleMember (reads require RoleMember+)", p.Role)
	}
	if p.ActorType != ActorTypeMCPAgent {
		t.Fatalf("ActorType = %q, want %q", p.ActorType, ActorTypeMCPAgent)
	}
}

func TestMCPPage_LimitClamping(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, mcpDefaultLimit},
		{-5, mcpDefaultLimit},
		{10, 10},
		{mcpMaxLimit, mcpMaxLimit},
		{mcpMaxLimit + 1, mcpMaxLimit},
	}
	for _, c := range cases {
		if got := (MCPPage{Limit: c.in}).limit(); got != c.want {
			t.Errorf("MCPPage{Limit:%d}.limit() = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPaginateContacts_SetsCursorAndTrims(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	c := MCPContacts{Data: []MCPContact{{ID: a}, {ID: b}, {ID: uuid.New()}}}
	paginateContacts(&c, 2)
	if len(c.Data) != 2 {
		t.Fatalf("len = %d, want 2", len(c.Data))
	}
	if !c.HasMore {
		t.Fatal("HasMore should be true")
	}
	if c.NextCursor == nil || *c.NextCursor != b.String() {
		t.Fatalf("NextCursor = %v, want %s", c.NextCursor, b)
	}
}

func TestPaginateContacts_NoMore(t *testing.T) {
	c := MCPContacts{Data: []MCPContact{{ID: uuid.New()}}}
	paginateContacts(&c, 50)
	if c.HasMore || c.NextCursor != nil {
		t.Fatal("single page should not report more")
	}
}

func TestPaginateDeals_EmptyNonNil(t *testing.T) {
	var d MCPDeals
	paginateDeals(&d, 50)
	if d.Data == nil {
		t.Fatal("empty page Data must serialise as [] not null")
	}
}

func TestMCPCursorArg(t *testing.T) {
	if mcpCursorArg(uuid.Nil) != nil {
		t.Fatal("nil uuid must map to NULL arg")
	}
	id := uuid.New()
	if got := mcpCursorArg(id); got == nil || *got != id {
		t.Fatalf("mcpCursorArg(%s) = %v", id, got)
	}
}
