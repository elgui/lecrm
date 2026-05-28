// Deals — sales-pipeline cube.
//
// Joins:
//   - PipelineStages on stage_id (LEFT JOIN: deals without a stage
//     still surface in totals).
//   - Contacts on contact_id.
//   - Companies on company_id.
//
// `deal_stage` dimension depends on the workspace_<id>.pipeline_stages
// table; it exists for any workspace provisioned with the
// `gbconsult-default` template (migration 0006) and is null otherwise.

cube('Deals', {
  sql: 'SELECT * FROM deals',

  joins: {
    PipelineStages: {
      relationship: 'belongsTo',
      sql: '${CUBE}.stage_id = ${PipelineStages}.id',
    },
    Contacts: {
      relationship: 'belongsTo',
      sql: '${CUBE}.contact_id = ${Contacts}.id',
    },
    Companies: {
      relationship: 'belongsTo',
      sql: '${CUBE}.company_id = ${Companies}.id',
    },
  },

  measures: {
    count: {
      type: 'count',
    },
    totalAmount: {
      sql: 'amount',
      type: 'sum',
      format: 'currency',
    },
    avgAmount: {
      sql: 'amount',
      type: 'avg',
      format: 'currency',
    },
    wonCount: {
      type: 'count',
      filters: [{ sql: '${CUBE}.closed_at IS NOT NULL' }],
    },
    openCount: {
      type: 'count',
      filters: [{ sql: '${CUBE}.closed_at IS NULL' }],
    },
  },

  dimensions: {
    id: {
      sql: 'id',
      type: 'string',
      primaryKey: true,
    },
    title: {
      sql: 'title',
      type: 'string',
    },
    amount: {
      sql: 'amount',
      type: 'number',
    },
    currency: {
      sql: 'currency',
      type: 'string',
    },
    stageId: {
      sql: 'stage_id',
      type: 'string',
    },
    // Resolved via the PipelineStages join — null for deals on a
    // workspace without the gbconsult-default pipeline seeded.
    dealStage: {
      sql: '${PipelineStages}.name',
      type: 'string',
      title: 'Stage',
    },
    ownerId: {
      sql: 'owner_id',
      type: 'string',
    },
    expectedCloseDate: {
      sql: 'expected_close_date',
      type: 'time',
    },
    closedAt: {
      sql: 'closed_at',
      type: 'time',
    },
    createdAt: {
      sql: 'created_at',
      type: 'time',
    },
  },
});
