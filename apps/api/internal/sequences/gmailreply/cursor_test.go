package gmailreply

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences"
)

func TestLoadCursor(t *testing.T) {
	ctx := context.Background()

	t.Run("present", func(t *testing.T) {
		tx := &fakeTx{cursorRaw: []byte(`{"history_id":9001}`)}
		hid, found, err := loadCursor(ctx, tx)
		if err != nil || !found || hid != 9001 {
			t.Fatalf("got (%d, %v, %v), want (9001, true, nil)", hid, found, err)
		}
	})

	t.Run("no connection row", func(t *testing.T) {
		tx := &fakeTx{cursorNoRow: true}
		_, found, err := loadCursor(ctx, tx)
		if err != nil || found {
			t.Fatalf("got (found=%v, err=%v), want (false, nil)", found, err)
		}
	})

	t.Run("null cursor", func(t *testing.T) {
		tx := &fakeTx{cursorRaw: []byte("null")}
		_, found, err := loadCursor(ctx, tx)
		if err != nil || found {
			t.Fatalf("got (found=%v, err=%v), want (false, nil)", found, err)
		}
	})

	t.Run("zero history id treated as unset", func(t *testing.T) {
		tx := &fakeTx{cursorRaw: []byte(`{"history_id":0}`)}
		_, found, err := loadCursor(ctx, tx)
		if err != nil || found {
			t.Fatalf("got (found=%v, err=%v), want (false, nil)", found, err)
		}
	})
}

func TestSaveCursor_PassesJSONBAsString(t *testing.T) {
	tx := &fakeTx{}
	if err := saveCursor(context.Background(), tx, 12345); err != nil {
		t.Fatalf("saveCursor: %v", err)
	}
	e, ok := tx.execMatching("UPDATE sync_connections")
	if !ok {
		t.Fatal("no UPDATE sync_connections exec recorded")
	}
	if !strings.Contains(e.sql, "$1::jsonb") {
		t.Errorf("save SQL should cast $1 to jsonb, got: %s", e.sql)
	}
	// First arg MUST be a string, not []byte (simple-protocol jsonb footgun).
	if _, isStr := e.args[0].(string); !isStr {
		t.Fatalf("cursor jsonb arg type = %T, want string", e.args[0])
	}
	if got := e.args[0].(string); !strings.Contains(got, `"history_id":12345`) {
		t.Errorf("cursor payload = %q", got)
	}
}

func TestSaveCursor_NoConnectionRow_Errors(t *testing.T) {
	tx := &fakeTx{saveZeroRows: true}
	if err := saveCursor(context.Background(), tx, 1); err == nil {
		t.Fatal("expected error when no gmail connection row exists")
	}
}

func TestMatchSteps(t *testing.T) {
	ctx := context.Background()

	t.Run("empty input short-circuits", func(t *testing.T) {
		// nil tx would nil-deref if Query were called — proves no query runs.
		got, err := matchSteps(ctx, &fakeTx{}, nil)
		if err != nil || got != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("maps rows to MatchedStep", func(t *testing.T) {
		enr := uuid.New()
		tx := &fakeTx{matched: []MatchedStep{
			{RFCMessageID: "a@x", EnrollmentID: enr, StepIndex: 3, State: sequences.StateWaitingReply},
		}}
		got, err := matchSteps(ctx, tx, []string{"a@x"})
		if err != nil {
			t.Fatalf("matchSteps: %v", err)
		}
		if len(got) != 1 || got[0].EnrollmentID != enr || got[0].StepIndex != 3 ||
			got[0].State != sequences.StateWaitingReply {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestListActiveGmailConnections(t *testing.T) {
	user := uuid.New()
	tx := &fakeTx{conns: []connRow{{
		id:       uuid.New(),
		settings: []byte(`{"user_id":"` + user.String() + `","email_address":"rep@example.com"}`),
		cursor:   []byte(`{"history_id":42}`),
	}}}
	got, err := listActiveGmailConnections(context.Background(), tx)
	if err != nil {
		t.Fatalf("listActiveGmailConnections: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d connections, want 1", len(got))
	}
	if got[0].UserID != user {
		t.Errorf("user = %s, want %s", got[0].UserID, user)
	}
	if got[0].EmailAddress != "rep@example.com" {
		t.Errorf("email = %q", got[0].EmailAddress)
	}
	if got[0].HistoryID != 42 {
		t.Errorf("cursor historyId = %d, want 42", got[0].HistoryID)
	}
}
