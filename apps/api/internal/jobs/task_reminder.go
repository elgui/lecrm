package jobs

import (
	"time"

	"github.com/google/uuid"
)

// TaskReminderJob is enqueued when a task has a due_date set so a
// reminder can be delivered on (or shortly before) that day. The job
// payload carries only the workspace + task ID so the worker can
// re-read the task fresh: a task that has been completed or deleted
// between scheduling and execution must NOT fire a reminder.
//
// Today the JobRunner implementation is a placeholder that only logs
// the Enqueue call (see river_adapter.go) — wiring real delivery
// (email/in-app/etc.) is Sprint 8+. The intent is recorded in the
// activity log on the originating mutation so the trail is durable
// even before delivery is implemented.
type TaskReminderJob struct {
	workspaceID uuid.UUID
	taskID      uuid.UUID
	due         time.Time
}

// NewTaskReminderJob constructs a reminder job for a task. `due` is the
// task's due_date (date-only — time-of-day is not modeled at v0).
func NewTaskReminderJob(workspaceID, taskID uuid.UUID, due time.Time) TaskReminderJob {
	return TaskReminderJob{workspaceID: workspaceID, taskID: taskID, due: due}
}

func (j TaskReminderJob) Kind() string           { return "task.reminder" }
func (j TaskReminderJob) WorkspaceID() uuid.UUID { return j.workspaceID }
func (j TaskReminderJob) TaskID() uuid.UUID      { return j.taskID }
func (j TaskReminderJob) DueDate() time.Time     { return j.due }
