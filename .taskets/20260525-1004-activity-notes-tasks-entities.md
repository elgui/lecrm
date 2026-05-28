---
id: 20260525-1004-activity-notes-tasks-entities
title: "Activity log + Notes + Tasks entities and handlers"
status: todo
priority: p1
created: 2026-05-25
category: project
group: crm-crud-complete
group_order: 60
order: 2
plan: true
tags: [crm, activity, notes, tasks, river, sprint-7]
---

# Activity log + Notes + Tasks entities and handlers

## Pre-flight: Verify Previous Tasket

Before starting, verify Tasket 4 ("Entity REST handlers") completed:

1. `ls apps/api/internal/http/contacts.go` -- handler exists
2. `cd apps/api && go test -race -count=1 ./internal/http/...` -- handler tests pass
3. `git log --oneline -10 | grep -i "REST handler\|CRUD"` -- commit exists

**If any check fails, STOP immediately and report. Do not proceed.**

## Context

Features 4 (notes/activity log) and 5 (tasks with due dates) from the v0 feature list. Activities provide an append-only timeline on every entity. Notes are user-authored text attached to entities. Tasks have due dates with River-scheduled reminders.

Sprint 7 work per `docs/sprint-plan.md`.

Source of truth: `docs/sprint-plan.md` Sprint 7
Working directory: `/home/gui/Projects/leCRM`

## Steps

1. Create migration for the three tables in workspace schema:
   ```sql
   -- Activities: append-only timeline
   CREATE TABLE activities (
     id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
     entity_type text NOT NULL CHECK (entity_type IN ('contact','company','deal')),
     entity_id uuid NOT NULL,
     actor_type text CHECK (actor_type IN ('human_api','mcp_agent','internal_service','system','connector')),
     actor_id uuid,
     event_type text NOT NULL,
     source_system text,  -- 'chatboting', 'gmail', 'manual', etc. (ADR-011)
     payload jsonb NOT NULL DEFAULT '{}',
     created_at timestamptz NOT NULL DEFAULT now()
   );
   CREATE INDEX activities_entity_idx ON activities (entity_type, entity_id, created_at DESC);

   -- Notes: user-authored text on entities
   CREATE TABLE notes (
     id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
     entity_type text NOT NULL CHECK (entity_type IN ('contact','company','deal')),
     entity_id uuid NOT NULL,
     body text NOT NULL,
     author_id uuid NOT NULL,
     created_at timestamptz NOT NULL DEFAULT now(),
     updated_at timestamptz NOT NULL DEFAULT now()
   );
   CREATE INDEX notes_entity_idx ON notes (entity_type, entity_id, created_at DESC);

   -- Tasks: actionable items with due dates
   CREATE TABLE tasks (
     id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
     title text NOT NULL,
     description text,
     entity_type text CHECK (entity_type IN ('contact','company','deal')),
     entity_id uuid,
     assignee_id uuid,
     due_date date,
     completed_at timestamptz,
     created_at timestamptz NOT NULL DEFAULT now(),
     updated_at timestamptz NOT NULL DEFAULT now()
   );
   CREATE INDEX tasks_assignee_due_idx ON tasks (assignee_id, due_date) WHERE completed_at IS NULL;
   ```

2. Write sqlc queries for each entity and generate Go code

3. Implement REST handlers:
   - Activities (read-only from REST — writes happen via internal service on entity mutations):
     - `GET /v1/contacts/:id/activities` — timeline for a contact
     - `GET /v1/deals/:id/activities` — timeline for a deal
   - Notes:
     - `GET /v1/contacts/:id/notes` — list notes on a contact
     - `POST /v1/contacts/:id/notes` — create note
     - `PUT /v1/notes/:id` — edit note (author only)
     - `DELETE /v1/notes/:id` — delete note (author or admin)
   - Tasks:
     - `GET /v1/tasks` — list tasks for current user (optionally filter by entity)
     - `POST /v1/tasks` — create task
     - `PUT /v1/tasks/:id` — update task
     - `PATCH /v1/tasks/:id/complete` — toggle completion
     - `DELETE /v1/tasks/:id` — delete task

4. Wire activity creation into entity mutation handlers:
   - Contact/Company/Deal create → activity `entity.created`
   - Contact/Company/Deal update → activity `entity.updated` with changed fields in payload
   - Deal stage change → activity `deal.stage_changed` with old_stage/new_stage
   - Note created → activity `note.added`
   - Connector events (ADR-011) → activity with `actor_type='connector'`, `source_system` populated

5. River-scheduled task reminders:
   - When a task has `due_date` set, enqueue a River job for the due date
   - Job checks if task is still incomplete, sends notification (placeholder for v0 — log only)
   - On task update/delete, cancel the scheduled job

6. Tests:
   - Test: activity timeline returns events in reverse chronological order
   - Test: activity created automatically on entity mutation
   - Test: notes CRUD lifecycle
   - Test: task completion toggle
   - Test: cross-tenant isolation on all three entities

## Done When

- [ ] Activities table populated automatically on entity mutations
- [ ] Activities include `actor_type` and `source_system` for connector attribution
- [ ] Notes CRUD works (create, read, update by author, delete by author/admin)
- [ ] Tasks CRUD works with due dates
- [ ] Task completion toggle works
- [ ] River job scheduled for task due dates
- [ ] Cross-tenant isolation verified
- [ ] All tests pass

## Completion Verification

1. `grep -c 'activities\|notes\|tasks' packages/db/queries/*.sql` -- query files exist
2. `cd apps/api && go test -race -count=1 ./...` -- all tests pass
3. Commit: `feat(api): Activity log, Notes, Tasks entities and handlers (Sprint 7)`

## References

- `apps/api/internal/http/contacts.go` — entity handlers to wire activities into
- `apps/api/internal/jobs/` — River job pattern for task reminders
- `docs/adr/ADR-011-chatboting-connector-boundary.md` §3c — connector actor_type
- `docs/sprint-plan.md` Sprint 7 features 4+5
