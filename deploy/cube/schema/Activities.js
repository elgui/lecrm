// Activities — sourced from the workspace `objects` table
// (ADR-010 §3 JSONB-primary storage) where object_type = 'activity'.
//
// v0 placeholder shape: dedicated `activities` table lands when the
// activity-log feature ships (post-Sprint-8). Until then the metadata-
// engine objects table holds activity rows with this canonical
// `data` JSONB structure:
//
//   {
//     "kind":       "note" | "call" | "email" | "stage_change",
//     "subject":    "<short label>",
//     "occurred_at":"<ISO-8601>",
//     "actor_id":   "<uuid>"
//   }
//
// When the dedicated table lands, swap the base `sql` and the
// dimension SQLs to point at the new columns — measures stay stable.

cube('Activities', {
  sql: "SELECT * FROM objects WHERE object_type = 'activity'",

  measures: {
    count: {
      type: 'count',
    },
  },

  dimensions: {
    id: {
      sql: 'id',
      type: 'string',
      primaryKey: true,
    },
    parentType: {
      sql: 'parent_type',
      type: 'string',
    },
    parentId: {
      sql: 'parent_id',
      type: 'string',
    },
    kind: {
      sql: "data->>'kind'",
      type: 'string',
    },
    subject: {
      sql: "data->>'subject'",
      type: 'string',
    },
    actorId: {
      sql: "data->>'actor_id'",
      type: 'string',
    },
    occurredAt: {
      sql: "(data->>'occurred_at')::timestamptz",
      type: 'time',
    },
    createdAt: {
      sql: 'created_at',
      type: 'time',
    },
  },
});
