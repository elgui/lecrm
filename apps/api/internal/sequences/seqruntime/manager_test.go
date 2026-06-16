package seqruntime

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/sequences/gmailreply"
)

func TestEnqueuer_NoClientForWorkspace(t *testing.T) {
	mgr := NewManager(ManagerConfig{}) // nothing started → empty client map
	enq := &Enqueuer{Manager: mgr}

	err := enq.EnqueuePollMailbox(context.Background(), gmailreply.PollMailboxArgs{
		WorkspaceID: uuid.New(),
		UserID:      uuid.New(),
	})
	if err == nil {
		t.Fatal("want error when no river client is registered for the workspace")
	}
}

func TestManager_ClientMissing(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	if _, ok := mgr.Client(uuid.New()); ok {
		t.Fatal("Client should report ok=false for an unknown workspace")
	}
}

func TestWorkspaceSchema(t *testing.T) {
	id := uuid.MustParse("12345678-90ab-cdef-1234-567890abcdef")
	if got, want := WorkspaceSchema(id), "workspace_1234567890abcdef1234567890abcdef"; got != want {
		t.Errorf("WorkspaceSchema = %q, want %q", got, want)
	}
}
