-- 0010_pgcrypto_to_core_schema.sql — relocate pgcrypto into the core schema.
--
-- Migration 0006 pins SECURITY DEFINER search_path to `core, pg_catalog`,
-- which is correct for CWE-89 mitigation. But gen_random_bytes() lives in
-- `public` (installed there by 0001_init.sql), making it invisible from
-- inside the provisioning functions. Moving the extension into `core`
-- resolves the conflict without weakening the search_path restriction.

BEGIN;

ALTER EXTENSION pgcrypto SET SCHEMA core;

COMMIT;
