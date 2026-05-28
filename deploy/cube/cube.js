// cube.js — runtime config for leCRM v0.
//
// Two things this file does:
//
//   1. Reads the embed JWT (verified by CUBEJS_API_SECRET) and pulls
//      `workspace_id` off `securityContext`. The token is minted by
//      apps/api POST /v1/reports/embed-token with a 5-minute TTL.
//
//   2. Uses `queryRewrite` to run `SET LOCAL ROLE workspace_<id>_ro`
//      on the underlying Postgres connection before every user query.
//      This is the enforcement boundary: even if the schema accidentally
//      references the wrong table, the DB role lacks privileges on
//      other workspaces' schemas — defense in depth alongside the
//      JWT-driven schema selection in the dimension/measure SQL.
//
// Tenancy model: ADR-001 schema-per-tenant. The `workspace_<id>` schema
// holds deals/contacts/companies/objects; the RO role grants only
// SELECT on that schema (see packages/db/migrations/0013_*).
//
// A note on multitenancy: Cube's `contextToOrchestratorId` keys the
// schema/preAggregation cache by tenant — a missing return would let
// one workspace's cached query bleed into another's response.
// `workspace_id` is the only legitimate key.

function workspaceIdFrom(securityContext) {
  const wsid = securityContext && securityContext.workspace_id;
  if (!wsid || typeof wsid !== 'string') {
    throw new Error('cube: securityContext.workspace_id is required');
  }
  // Defense: only allow canonical UUID format. Anything else gets
  // rejected before it can be string-concatenated into SQL.
  if (!/^[0-9a-f-]{36}$/i.test(wsid)) {
    throw new Error('cube: securityContext.workspace_id must be a UUID');
  }
  return wsid;
}

function schemaNameFor(workspaceId) {
  return 'workspace_' + workspaceId.replace(/-/g, '').toLowerCase();
}

module.exports = {
  // Cache key — one orchestrator (and pre-agg cache) per workspace.
  contextToOrchestratorId: ({ securityContext }) => {
    return 'ws_' + workspaceIdFrom(securityContext);
  },

  // Cache key — one schema-compilation entry per workspace. Even though
  // all workspaces share the same Cube schema files today, this keeps
  // the door open for per-workspace custom dimensions later.
  contextToAppId: ({ securityContext }) => {
    return 'app_' + workspaceIdFrom(securityContext);
  },

  // Inject the workspace schema as a SQL `search_path` switch before
  // every query. We do NOT have to mutate the query AST — Cube schemas
  // reference tables unqualified, so search_path is the binding point.
  //
  // The `SET LOCAL ROLE` line is the real isolation. search_path is a
  // hint; the role privilege is the wall.
  queryRewrite: (query, { securityContext }) => {
    const workspaceId = workspaceIdFrom(securityContext);
    const roRoleName = schemaNameFor(workspaceId) + '_ro';
    // Cube exposes per-query connection setup via `query.queryHooks`.
    // We push a pre-query SQL list that runs inside the same transaction.
    query.preQueries = (query.preQueries || []).concat([
      // SET LOCAL ROLE keeps the role change scoped to the transaction;
      // pool checkout returns to lecrm_cube_reader on next acquire.
      'SET LOCAL ROLE "' + roRoleName + '"',
    ]);
    return query;
  },

  // Tighten the default scheduled refresh — the v0 baseline dashboards
  // do not need sub-minute freshness. 5 minutes also matches the embed
  // token TTL, so the user's session can outlive any stale pre-agg.
  scheduledRefreshTimer: 5 * 60,
};
