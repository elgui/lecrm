package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestRoleName_MatchesProvisioningConvention(t *testing.T) {
	// Must match core.lecrm_provision_workspace:
	//   'workspace_' || lower(replace(uuid::text,'-','')) || '_ro'
	id := uuid.MustParse("0a1b2c3d-4e5f-6071-8293-a4b5c6d7e8f9")
	got := RoleName(id)
	want := "workspace_0a1b2c3d4e5f60718293a4b5c6d7e8f9_ro"
	if got != want {
		t.Fatalf("RoleName = %q, want %q", got, want)
	}
}

func TestRoleName_OnlySafeChars(t *testing.T) {
	got := RoleName(uuid.New())
	for _, c := range got {
		ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || c == '_'
		if !ok {
			t.Fatalf("RoleName produced unsafe char %q in %q", c, got)
		}
	}
}

func TestPage_LimitClamping(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, defaultLimit},
		{-5, defaultLimit},
		{10, 10},
		{maxLimit, maxLimit},
		{maxLimit + 1, maxLimit},
	}
	for _, c := range cases {
		if got := (Page{Limit: c.in}).limit(); got != c.want {
			t.Errorf("Page{Limit:%d}.limit() = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestContacts_PaginateSetsCursorAndTrims(t *testing.T) {
	// limit=2, 3 rows present → trim to 2, cursor = id of 2nd row, hasMore.
	a, b := uuid.New(), uuid.New()
	c := Contacts{Data: []Contact{{ID: a}, {ID: b}, {ID: uuid.New()}}}
	c.paginate(2)
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

func TestContacts_PaginateNoMore(t *testing.T) {
	c := Contacts{Data: []Contact{{ID: uuid.New()}}}
	c.paginate(50)
	if c.HasMore || c.NextCursor != nil {
		t.Fatal("single page should not report more")
	}
}

func TestDeals_PaginateEmptyNonNil(t *testing.T) {
	var d Deals
	d.paginate(50)
	if d.Data == nil {
		t.Fatal("empty page Data must serialise as [] not null")
	}
}

func TestCursorArg(t *testing.T) {
	if cursorArg(uuid.Nil) != nil {
		t.Fatal("nil uuid must map to NULL arg")
	}
	id := uuid.New()
	if got := cursorArg(id); got == nil || *got != id {
		t.Fatalf("cursorArg(%s) = %v", id, got)
	}
}
