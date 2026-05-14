-- 0002_identity.sql — global identity registry keyed on (issuer, sub).
--
-- Per ADR-009 §7.1 (binding): users are identified by the tuple
-- (issuer, sub), not by raw `sub`. Authentik issues UUID-format `sub`;
-- Zitadel issues numeric strings. Persisting both columns turns the
-- v0→v1 IDP migration from a destructive rewrite into a mapping table.
--
-- Users live in the global `core.users` table. Workspace membership is
-- recorded in `core.workspace_members` so a single human can belong to
-- multiple workspaces under the same identity. This matches Authentik's
-- multi-tenant convention where each tenant is an OAuth client and the
-- same `(issuer, sub)` resolves to a single human.
--
-- The application role does not own these tables (only the provisioner
-- does); read/write access for `lecrm-api` is granted explicitly in a
-- later migration when the application role is created.

BEGIN;

CREATE TABLE IF NOT EXISTS core.users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  issuer        TEXT        NOT NULL,                       -- OIDC `iss` claim
  subject       TEXT        NOT NULL,                       -- OIDC `sub` claim
  email         TEXT,                                       -- claim, not unique
  display_name  TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ,
  CONSTRAINT users_issuer_subject_uk UNIQUE (issuer, subject)
);

-- Lookup by email is a UX nicety (admin search), not an identity key.
CREATE INDEX IF NOT EXISTS users_email_idx ON core.users (lower(email));

CREATE TABLE IF NOT EXISTS core.workspace_members (
  workspace_id UUID NOT NULL REFERENCES core.workspaces(id) ON DELETE CASCADE,
  user_id      UUID NOT NULL REFERENCES core.users(id)      ON DELETE CASCADE,
  role         TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
  invited_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  joined_at    TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX IF NOT EXISTS workspace_members_user_idx ON core.workspace_members (user_id);

ALTER TABLE core.users              OWNER TO lecrm_provisioner;
ALTER TABLE core.workspace_members  OWNER TO lecrm_provisioner;

COMMIT;
