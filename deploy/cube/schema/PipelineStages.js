// PipelineStages — workspace-scoped sales-pipeline stages.
//
// Seeded only for workspaces created with the `gbconsult-default`
// template (migration 0006). For workspaces without the template,
// the table is absent and Cube queries that touch it will error —
// dashboards that depend on this cube should be gated in 12b.

cube('PipelineStages', {
  sql: 'SELECT * FROM pipeline_stages',

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
    name: {
      sql: 'name',
      type: 'string',
    },
    orderIndex: {
      sql: 'order_index',
      type: 'number',
    },
  },
});
