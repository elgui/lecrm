-- workspace_crm.sql — sqlc type-hint schema for workspace-scoped CRM tables.
--
-- These tables are created dynamically inside each workspace_<uuid> schema by
-- core.lecrm_provision_workspace (migration 0008). This file provides sqlc
-- with parseable DDL so it can generate typed Go code for CRUD queries.
--
-- NOT applied to the database directly — only consumed by sqlc.

CREATE TABLE IF NOT EXISTS pipeline_stages (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL UNIQUE,
  order_index INT  NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS companies (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name       TEXT NOT NULL,
  domain     TEXT,
  industry   TEXT,
  size       TEXT CHECK (size IN ('1-10','11-50','51-200','201-1000','1000+')),
  owner_id   UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS contacts (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  first_name  TEXT NOT NULL,
  last_name   TEXT NOT NULL,
  email       TEXT,
  phone       TEXT,
  company_id  UUID REFERENCES companies(id) ON DELETE SET NULL,
  owner_id    UUID,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS deals (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title               TEXT NOT NULL,
  amount              NUMERIC(12,2),
  currency            CHAR(3) DEFAULT 'EUR',
  stage_id            UUID REFERENCES pipeline_stages(id),
  contact_id          UUID REFERENCES contacts(id) ON DELETE SET NULL,
  company_id          UUID REFERENCES companies(id) ON DELETE SET NULL,
  owner_id            UUID,
  expected_close_date DATE,
  closed_at           TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS activities (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_type   TEXT NOT NULL,
  entity_id     UUID NOT NULL,
  actor_type    TEXT,
  actor_id      UUID,
  event_type    TEXT NOT NULL,
  source_system TEXT,
  payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notes (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_type TEXT NOT NULL,
  entity_id   UUID NOT NULL,
  body        TEXT NOT NULL,
  author_id   UUID NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tasks (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title        TEXT NOT NULL,
  description  TEXT,
  entity_type  TEXT,
  entity_id    UUID,
  assignee_id  UUID,
  due_date     DATE,
  completed_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- v1 native sequences (ADR-004 rev 2 §1). Created per-workspace by
-- core.lecrm_provision_workspace (migration 0025) as schema-qualified types +
-- tables; mirrored here unqualified so sqlc can emit typed Go bindings.
CREATE TYPE enrollment_state AS ENUM (
  'enrolled',
  'step_sent',
  'waiting_reply',
  'reply_received',
  'ooo_detected',
  'failed',
  'bounced',
  'unsubscribed',
  'suppressed',
  'completed'
);

CREATE TYPE step_send_state AS ENUM (
  'pending',
  'sent',
  'delivered',
  'bounced',
  'cancelled',
  'superseded'
);

CREATE TABLE IF NOT EXISTS enrollments (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sequence_id        UUID NOT NULL,
  contact_id         UUID NOT NULL,
  state              enrollment_state NOT NULL DEFAULT 'enrolled',
  current_step_index SMALLINT NOT NULL DEFAULT 0,
  enrolled_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  next_action_at     TIMESTAMPTZ,
  last_transition_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reply_message_id   TEXT,
  ooo_returns_at     TIMESTAMPTZ,
  created_by_user_id UUID,
  workspace_id       UUID NOT NULL
);

CREATE TABLE IF NOT EXISTS enrollment_steps (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  enrollment_id    UUID NOT NULL REFERENCES enrollments(id) ON DELETE CASCADE,
  step_index       SMALLINT NOT NULL,
  state            step_send_state NOT NULL DEFAULT 'pending',
  brevo_message_id TEXT,
  rfc_message_id   TEXT,
  scheduled_for    TIMESTAMPTZ NOT NULL,
  sent_at          TIMESTAMPTZ,
  delivered_at     TIMESTAMPTZ,
  bounced_at       TIMESTAMPTZ,
  bounce_type      TEXT,
  idempotency_key  TEXT NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
