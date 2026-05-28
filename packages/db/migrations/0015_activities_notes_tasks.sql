-- 0015_activities_notes_tasks.sql — Sprint 7 features 4+5
-- (tasket 20260525-1004): activities (append-only timeline), notes
-- (user-authored text), and tasks (actionable items with due dates).
--
-- All three tables live in each workspace_<uuid> schema (ADR-001
-- schema-per-tenant). The provisioning function is extended with
-- steps 16/17/18 and back-filled across all existing workspaces.
--
-- activities is the canonical timeline for downstream consumers
-- (chat UI, Kanban side-panel, audit-replay debugging). REST writes
-- happen inside the same writeTx as the entity mutation so fail-closed
-- semantics are preserved: a failed activity insert rolls back the
-- mutation, mirroring the audit_log contract (ADR-009 §7.2).
--
-- actor_type catalogue:
--   - human_api        REST writes via a session-authenticated user
--   - mcp_agent        MCP tool calls (ADR-011 §3c)
--   - internal_service in-process automations (cron, river jobs)
--   - system           database-level / migration writes
--   - connector        external sync workers (chatboting, gmail, ...)
--                      — `source_system` MUST be populated for these.
--
-- References:
--   docs/sprint-plan.md — Sprint 7 features 4 and 5
--   docs/adr/ADR-011-chatboting-connector-boundary.md §3c — actor attribution
--   packages/db/migrations/0013_workspace_ro_role.sql — previous function body

BEGIN;

CREATE OR REPLACE FUNCTION core.lecrm_provision_workspace(p_workspace_id UUID)
  RETURNS TEXT
  LANGUAGE plpgsql
  SECURITY DEFINER
  SET search_path = core, pg_catalog
AS $func$
DECLARE
  v_role_name    TEXT;
  v_ro_role_name TEXT;
  v_password     TEXT;
  v_river_name   TEXT;
BEGIN
  IF p_workspace_id IS NULL THEN
    RAISE EXCEPTION 'p_workspace_id must not be NULL'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF p_workspace_id = '00000000-0000-0000-0000-000000000000'::uuid THEN
    RAISE EXCEPTION 'p_workspace_id must not be the zero UUID'
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  v_role_name    := 'workspace_' || lower(replace(p_workspace_id::text, '-', ''));
  v_ro_role_name := v_role_name || '_ro';
  v_password     := encode(gen_random_bytes(32), 'base64');
  v_river_name   := 'river_' || lower(replace(p_workspace_id::text, '-', ''));

  -- 1. Owner role.
  BEGIN
    EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L CONNECTION LIMIT 10', v_role_name, v_password);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_provisioner', v_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  EXECUTE format('ALTER ROLE %I SET search_path = %I, public', v_role_name, v_role_name);
  EXECUTE format('ALTER ROLE %I SET statement_timeout = ''30s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET lock_timeout = ''5s''', v_role_name);
  EXECUTE format('ALTER ROLE %I SET work_mem = ''16MB''', v_role_name);

  -- 2. Schema.
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_role_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- 3. Owner grants.
  EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO %I', v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON TABLES TO %I',
    v_role_name, v_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES IN SCHEMA %I GRANT ALL ON SEQUENCES TO %I',
    v_role_name, v_role_name);

  -- 4. Lateral-expansion mitigation.
  EXECUTE format('REVOKE CREATE ON SCHEMA public FROM %I', v_role_name);
  EXECUTE format('REVOKE ALL ON ALL TABLES IN SCHEMA public FROM %I', v_role_name);

  -- 5. River-job schema.
  BEGIN
    EXECUTE format('CREATE SCHEMA %I AUTHORIZATION %I', v_river_name, v_role_name);
  EXCEPTION
    WHEN duplicate_schema THEN NULL;
  END;

  -- 6. Custom-property metadata table (ADR-010 §4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.custom_property_definitions (
      id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      parent_type     text NOT NULL CHECK (parent_type IN ('contact', 'deal')),
      property_key    text NOT NULL,
      property_type   text NOT NULL CHECK (property_type IN ('string', 'number', 'boolean', 'enum', 'date')),
      allowed_values  jsonb NULL,
      required        boolean NOT NULL DEFAULT false,
      created_at      timestamptz NOT NULL DEFAULT now(),
      UNIQUE (parent_type, property_key)
    )$f$, v_role_name);

  -- 7. JSONB-primary object storage table + indexes (ADR-010 §3).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.objects (
      id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      object_type   text NOT NULL,
      parent_type   text NULL,
      parent_id     uuid NULL,
      data          jsonb NOT NULL DEFAULT '{}'::jsonb,
      created_at    timestamptz NOT NULL DEFAULT now(),
      updated_at    timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS objects_type_parent_idx ON %I.objects (object_type, parent_type, parent_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS objects_data_gin_idx ON %I.objects USING gin (data)',
    v_role_name);

  -- 8. Companies table.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.companies (
      id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      name       text NOT NULL,
      domain     text,
      industry   text,
      size       text CHECK (size IN ('1-10','11-50','51-200','201-1000','1000+')),
      owner_id   uuid,
      created_at timestamptz NOT NULL DEFAULT now(),
      updated_at timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  -- 9. Contacts table.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.contacts (
      id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      first_name  text NOT NULL,
      last_name   text NOT NULL,
      email       text,
      phone       text,
      company_id  uuid REFERENCES %I.companies(id) ON DELETE SET NULL,
      owner_id    uuid,
      created_at  timestamptz NOT NULL DEFAULT now(),
      updated_at  timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS contacts_company_id_idx ON %I.contacts (company_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS contacts_email_idx ON %I.contacts (email) WHERE email IS NOT NULL',
    v_role_name);

  -- 10. Deals table.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.deals (
      id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      title               text NOT NULL,
      amount              numeric(12,2),
      currency            char(3) DEFAULT 'EUR',
      stage_id            uuid,
      contact_id          uuid REFERENCES %I.contacts(id) ON DELETE SET NULL,
      company_id          uuid REFERENCES %I.companies(id) ON DELETE SET NULL,
      owner_id            uuid,
      expected_close_date date,
      closed_at           timestamptz,
      created_at          timestamptz NOT NULL DEFAULT now(),
      updated_at          timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name, v_role_name, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS deals_stage_id_idx ON %I.deals (stage_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS deals_contact_id_idx ON %I.deals (contact_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS deals_expected_close_date_idx ON %I.deals (expected_close_date) WHERE expected_close_date IS NOT NULL',
    v_role_name);

  -- 11. Sync connections — per-tenant provider lifecycle (ADR-011 §3).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.sync_connections (
      id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      provider_id  text NOT NULL,
      status       text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','active','paused','error','revoked','disconnected')),
      settings     jsonb NOT NULL DEFAULT '{}'::jsonb,
      sync_cursor  jsonb,
      last_sync_at timestamptz,
      last_error   text,
      created_at   timestamptz NOT NULL DEFAULT now(),
      updated_at   timestamptz NOT NULL DEFAULT now(),
      UNIQUE (provider_id)
    )$f$, v_role_name);

  -- 12. External entity mappings — external ID ↔ leCRM entity (ADR-011 §4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.external_entity_mappings (
      id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      provider_id    text NOT NULL,
      external_id    text NOT NULL,
      entity_type    text NOT NULL,
      entity_id      uuid NOT NULL,
      last_synced_at timestamptz NOT NULL DEFAULT now(),
      meta           jsonb NOT NULL DEFAULT '{}'::jsonb,
      created_at     timestamptz NOT NULL DEFAULT now(),
      UNIQUE (provider_id, external_id)
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS ext_map_entity_idx ON %I.external_entity_mappings (entity_type, entity_id)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS ext_map_provider_idx ON %I.external_entity_mappings (provider_id, external_id)',
    v_role_name);

  -- 13. Email suppression list — ADR-003 §Mitigations item 3.
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.email_suppression (
      id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      email          text NOT NULL UNIQUE,
      reason         text NOT NULL CHECK (reason IN ('hard_bounce','blocked','complaint','unsubscribed')),
      suppressed_at  timestamptz NOT NULL DEFAULT now(),
      created_at     timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS email_suppression_at_idx ON %I.email_suppression (suppressed_at DESC)',
    v_role_name);

  -- 14. Email send log — 7-day rolling bounce-rate alarm (ADR-003 §Mitigations item 4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.email_send_log (
      id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      message_id      text,
      recipient_email text NOT NULL,
      sent_at         timestamptz NOT NULL DEFAULT now(),
      bounced_at      timestamptz,
      bounce_kind     text CHECK (bounce_kind IN ('hard_bounce','complaint'))
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS email_send_log_sent_at_idx ON %I.email_send_log (sent_at DESC)',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS email_send_log_message_id_idx ON %I.email_send_log (message_id) WHERE message_id IS NOT NULL',
    v_role_name);

  -- 15. Per-workspace READ-ONLY role for Cube.dev (ADR-009 §9).
  BEGIN
    EXECUTE format('CREATE ROLE %I NOLOGIN', v_ro_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_provisioner', v_ro_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;
  BEGIN
    EXECUTE format('GRANT %I TO lecrm_cube_reader', v_ro_role_name);
  EXCEPTION
    WHEN duplicate_object THEN NULL;
  END;

  EXECUTE format('ALTER ROLE %I SET search_path = %I, core, pg_catalog', v_ro_role_name, v_role_name);
  EXECUTE format('ALTER ROLE %I SET statement_timeout = ''15s''', v_ro_role_name);
  EXECUTE format('ALTER ROLE %I SET lock_timeout = ''2s''', v_ro_role_name);

  EXECUTE format('GRANT USAGE ON SCHEMA %I TO %I', v_role_name, v_ro_role_name);
  EXECUTE format('GRANT SELECT ON ALL TABLES IN SCHEMA %I TO %I', v_role_name, v_ro_role_name);
  EXECUTE format('GRANT SELECT ON ALL SEQUENCES IN SCHEMA %I TO %I', v_role_name, v_ro_role_name);

  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA %I GRANT SELECT ON TABLES TO %I',
    v_role_name, v_role_name, v_ro_role_name);
  EXECUTE format(
    'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA %I GRANT SELECT ON SEQUENCES TO %I',
    v_role_name, v_role_name, v_ro_role_name);

  EXECUTE format('REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA %I FROM %I',
    v_role_name, v_ro_role_name);

  -- 16. Activities — append-only entity timeline (Sprint 7 feature 4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.activities (
      id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      entity_type   text NOT NULL CHECK (entity_type IN ('contact','company','deal')),
      entity_id     uuid NOT NULL,
      actor_type    text CHECK (actor_type IN ('human_api','mcp_agent','internal_service','system','connector')),
      actor_id      uuid,
      event_type    text NOT NULL,
      source_system text,
      payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
      created_at    timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS activities_entity_idx ON %I.activities (entity_type, entity_id, created_at DESC)',
    v_role_name);

  -- 17. Notes — user-authored text on entities (Sprint 7 feature 4).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.notes (
      id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      entity_type text NOT NULL CHECK (entity_type IN ('contact','company','deal')),
      entity_id   uuid NOT NULL,
      body        text NOT NULL,
      author_id   uuid NOT NULL,
      created_at  timestamptz NOT NULL DEFAULT now(),
      updated_at  timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS notes_entity_idx ON %I.notes (entity_type, entity_id, created_at DESC)',
    v_role_name);

  -- 18. Tasks — actionable items with due dates (Sprint 7 feature 5).
  EXECUTE format($f$
    CREATE TABLE IF NOT EXISTS %I.tasks (
      id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      title        text NOT NULL,
      description  text,
      entity_type  text CHECK (entity_type IN ('contact','company','deal')),
      entity_id    uuid,
      assignee_id  uuid,
      due_date     date,
      completed_at timestamptz,
      created_at   timestamptz NOT NULL DEFAULT now(),
      updated_at   timestamptz NOT NULL DEFAULT now()
    )$f$, v_role_name);

  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS tasks_assignee_due_idx ON %I.tasks (assignee_id, due_date) WHERE completed_at IS NULL',
    v_role_name);
  EXECUTE format(
    'CREATE INDEX IF NOT EXISTS tasks_entity_idx ON %I.tasks (entity_type, entity_id) WHERE entity_id IS NOT NULL',
    v_role_name);

  -- Refresh RO-role grants so the new tables are SELECT-able via Cube.
  -- ALTER DEFAULT PRIVILEGES (step 15) only applies to objects created
  -- AFTER it ran; tables added in this same call need an explicit GRANT.
  EXECUTE format('GRANT SELECT ON %I.activities TO %I', v_role_name, v_ro_role_name);
  EXECUTE format('GRANT SELECT ON %I.notes      TO %I', v_role_name, v_ro_role_name);
  EXECUTE format('GRANT SELECT ON %I.tasks      TO %I', v_role_name, v_ro_role_name);

  RETURN v_role_name;
END;
$func$;

ALTER FUNCTION core.lecrm_provision_workspace(UUID) OWNER TO lecrm_provisioner;
REVOKE ALL ON FUNCTION core.lecrm_provision_workspace(UUID) FROM PUBLIC;

-- Backfill: provision activities/notes/tasks for existing workspaces.
DO $$
DECLARE
  ws RECORD;
BEGIN
  FOR ws IN SELECT id FROM core.workspaces LOOP
    PERFORM core.lecrm_provision_workspace(ws.id);
  END LOOP;
END$$;

COMMIT;
