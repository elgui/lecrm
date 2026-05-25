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
