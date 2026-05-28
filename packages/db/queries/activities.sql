-- activities.sql — workspace-scoped activity timeline queries.
--
-- All queries run with search_path = workspace_<uuid>; table names are
-- unqualified. The REST surface is read-only — writes happen inside
-- the same transaction as the entity mutation via the raw helper in
-- apps/api/internal/crm/activity.go (fail-closed: a failed activity
-- insert rolls back the mutation, mirroring ADR-009 §7.2).

-- name: ListActivitiesByEntity :many
SELECT id, entity_type, entity_id, actor_type, actor_id, event_type,
       source_system, payload, created_at
FROM activities
WHERE entity_type = sqlc.arg('entity_type')::text
  AND entity_id   = sqlc.arg('entity_id')::uuid
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg('page_limit')::int;
