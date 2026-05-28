// Companies — accounts in the CRM.

cube('Companies', {
  sql: 'SELECT * FROM companies',

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
    domain: {
      sql: 'domain',
      type: 'string',
    },
    industry: {
      sql: 'industry',
      type: 'string',
    },
    size: {
      sql: 'size',
      type: 'string',
    },
    ownerId: {
      sql: 'owner_id',
      type: 'string',
    },
    createdAt: {
      sql: 'created_at',
      type: 'time',
    },
  },
});
