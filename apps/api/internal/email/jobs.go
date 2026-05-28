package email

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/gbconsult/lecrm/apps/api/internal/email/brevo"
	"github.com/gbconsult/lecrm/apps/api/internal/jobs"
)

// JobKindSendRequested is the river job kind for the outbound mutation
// path. The handler resolves the workspace, opens a workspace-scoped
// connection (jobs.RunWorkspaceJob), and calls Service.Send.
//
// Naming convention: dot-separated, matches the audit-log event names
// per ADR-009 §8.3 "Per-workspace river_<workspace_base36> schema". The
// river adapter routes jobs by kind.
const JobKindSendRequested = "email.send.requested"

// JobKindEventReceived is the river job kind for the inbound-webhook
// ingestion path. The HTTP webhook handler enqueues one of these per
// incoming event, the worker calls Service.IngestEvent.
const JobKindEventReceived = "email.event.received"

// SendJob is the river-job payload for a queued outbound send.
type SendJob struct {
	jobs.SimpleJob
	Request SendRequest
}

// NewSendJob constructs a SendJob targeting the workspace named in req.
func NewSendJob(req SendRequest) SendJob {
	return SendJob{
		SimpleJob: jobs.NewSimpleJob(JobKindSendRequested, req.WorkspaceID),
		Request:   req,
	}
}

// EventJob is the river-job payload for an inbound webhook event.
type EventJob struct {
	jobs.SimpleJob
	Schema string
	Event  brevo.Event
}

// NewEventJob constructs an EventJob for the given workspace + parsed
// webhook event.
func NewEventJob(workspaceID uuid.UUID, schema string, ev brevo.Event) EventJob {
	return EventJob{
		SimpleJob: jobs.NewSimpleJob(JobKindEventReceived, workspaceID),
		Schema:    schema,
		Event:     ev,
	}
}

// HandleSend is the worker-side function the river adapter calls for a
// SendJob. Wrapped by jobs.RunWorkspaceJob upstream, which acquires the
// per-workspace advisory lock and verifies search_path before this is
// invoked.
func (s *Service) HandleSend(ctx context.Context, j SendJob) error {
	if j.WorkspaceID() == uuid.Nil {
		return fmt.Errorf("email: HandleSend: nil workspace id")
	}
	_, err := s.Send(ctx, j.Request)
	return err
}

// HandleEvent is the worker-side function for an EventJob.
func (s *Service) HandleEvent(ctx context.Context, j EventJob) error {
	if j.WorkspaceID() == uuid.Nil {
		return fmt.Errorf("email: HandleEvent: nil workspace id")
	}
	return s.IngestEvent(ctx, j.WorkspaceID(), j.Schema, j.Event)
}
