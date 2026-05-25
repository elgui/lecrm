package jobs

import (
	"context"

	"github.com/google/uuid"
)

// Job describes a unit of background work scoped to a workspace.
type Job interface {
	Kind() string
	WorkspaceID() uuid.UUID
}

// SimpleJob is a minimal Job implementation for enqueuing work.
type SimpleJob struct {
	kind        string
	workspaceID uuid.UUID
}

// NewSimpleJob creates a Job with the given kind and workspace scope.
func NewSimpleJob(kind string, workspaceID uuid.UUID) SimpleJob {
	return SimpleJob{kind: kind, workspaceID: workspaceID}
}

func (j SimpleJob) Kind() string           { return j.kind }
func (j SimpleJob) WorkspaceID() uuid.UUID { return j.workspaceID }

// JobRunner abstracts the background job system. Business logic depends
// on this interface, not on the underlying implementation (River,
// Temporal, etc.). If the job system needs to be replaced, only the
// adapter changes.
type JobRunner interface {
	Enqueue(ctx context.Context, job Job) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
