// Contacts — people in the CRM.
//
// Joins to Companies via company_id.

cube('Contacts', {
  sql: 'SELECT * FROM contacts',

  joins: {
    Companies: {
      relationship: 'belongsTo',
      sql: '${CUBE}.company_id = ${Companies}.id',
    },
  },

  measures: {
    count: {
      type: 'count',
    },
    withEmail: {
      type: 'count',
      filters: [{ sql: '${CUBE}.email IS NOT NULL' }],
    },
  },

  dimensions: {
    id: {
      sql: 'id',
      type: 'string',
      primaryKey: true,
    },
    firstName: {
      sql: 'first_name',
      type: 'string',
    },
    lastName: {
      sql: 'last_name',
      type: 'string',
    },
    email: {
      sql: 'email',
      type: 'string',
    },
    phone: {
      sql: 'phone',
      type: 'string',
    },
    ownerId: {
      sql: 'owner_id',
      type: 'string',
    },
    companyId: {
      sql: 'company_id',
      type: 'string',
    },
    createdAt: {
      sql: 'created_at',
      type: 'time',
    },
  },
});
