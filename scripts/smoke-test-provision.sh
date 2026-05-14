#!/usr/bin/env bash
# smoke-test-provision.sh — verify packages/db/migrations/0001_init.sql.
#
# Boots an isolated, user-owned Postgres cluster on a non-privileged port,
# applies the bootstrap migration, then exercises the
# `core.lecrm_provision_workspace(uuid)` function against the acceptance
# criteria from ADR-009 §2.1:
#
#   1. Provision workspace_acme — verify the role, the schema, the
#      per-tenant river_* schema, and the search_path inheritance.
#   2. Re-call with the same UUID — must succeed and return the same
#      role name (idempotency).
#   3. Connect AS the workspace role — `SHOW search_path` must return
#      `workspace_<id>, public`.
#   4. Cross-check lateral-expansion mitigation — workspace role must NOT
#      have CREATE on public.
#
# Default: spin up Postgres 15 from /usr/lib/postgresql/15/bin. The prod
# target is Postgres 17 per ADR-009 §2; the function body uses only
# portable features (gen_random_bytes from pgcrypto, EXECUTE format,
# duplicate_object/duplicate_schema exception classes) that work on 13+.
#
# Override PGBIN to point at a different cluster binary set.

set -euo pipefail

PGBIN="${PGBIN:-/usr/lib/postgresql/15/bin}"
PGPORT="${PGPORT:-54329}"
WORKDIR="$(mktemp -d -t lecrm-smoke-XXXXXX)"
PGDATA="${WORKDIR}/data"
PGSOCK="${WORKDIR}/sock"
LOGFILE="${WORKDIR}/postgres.log"
TEST_UUID="aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
TEST_ROLE="workspace_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
TEST_RIVER="river_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

cleanup() {
  if [[ -d "${PGDATA}" ]]; then
    "${PGBIN}/pg_ctl" -D "${PGDATA}" -m fast stop >/dev/null 2>&1 || true
  fi
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

step() { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
pass() { printf '   \033[1;32mOK\033[0m %s\n' "$*"; }
fail() { printf '   \033[1;31mFAIL\033[0m %s\n' "$*"; exit 1; }

# --- 1. Cluster ---
step "Initializing temporary Postgres cluster at ${PGDATA}"
mkdir -p "${PGSOCK}"
"${PGBIN}/initdb" -D "${PGDATA}" --auth-host=trust --auth-local=trust --username=postgres --encoding=UTF8 >/dev/null

step "Starting Postgres on port ${PGPORT} (socket=${PGSOCK})"
"${PGBIN}/pg_ctl" -D "${PGDATA}" -l "${LOGFILE}" -o "-p ${PGPORT} -k ${PGSOCK} -h ''" start >/dev/null
export PGHOST="${PGSOCK}"
export PGPORT
export PGUSER=postgres
export PGDATABASE=postgres

# --- 2. Apply migrations in order ---
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATIONS_DIR="${SCRIPT_DIR}/../packages/db/migrations"
for m in "${MIGRATIONS_DIR}"/*.sql; do
  step "Applying $(basename "${m}")"
  psql -v ON_ERROR_STOP=1 -q -f "${m}" >/dev/null
done
pass "All migrations applied"

# Verify the function exists and is owned correctly.
OWNER=$(psql -tAc "SELECT pg_get_userbyid(p.proowner) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname='core' AND p.proname='lecrm_provision_workspace';")
[[ "${OWNER}" == "lecrm_provisioner" ]] || fail "function owner is '${OWNER}', expected 'lecrm_provisioner'"
pass "function owned by lecrm_provisioner"

# --- 3. First provisioning call ---
step "First call: lecrm_provision_workspace('${TEST_UUID}')"
# psql -tA still emits the 'SET' command tag for SET ROLE; strip everything
# but the last line so we keep only the function's return value.
RESULT1=$(psql -tA -c "SET ROLE lecrm_provisioner; SELECT core.lecrm_provision_workspace('${TEST_UUID}'::uuid);" | tail -n1)
[[ "${RESULT1}" == "${TEST_ROLE}" ]] || fail "expected ${TEST_ROLE}, got ${RESULT1}"
pass "returned role name = ${RESULT1}"

# Verify the role, schemas, and search_path attribute.
psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${TEST_ROLE}'" | grep -q 1 || fail "role ${TEST_ROLE} not created"
pass "role exists"
psql -tAc "SELECT 1 FROM pg_namespace WHERE nspname='${TEST_ROLE}'" | grep -q 1 || fail "schema ${TEST_ROLE} not created"
pass "workspace schema exists"
psql -tAc "SELECT 1 FROM pg_namespace WHERE nspname='${TEST_RIVER}'" | grep -q 1 || fail "schema ${TEST_RIVER} not created"
pass "river schema exists"
ROLE_PATH=$(psql -tAc "SELECT array_to_string(setconfig, ',') FROM pg_db_role_setting s JOIN pg_roles r ON r.oid = s.setrole WHERE r.rolname='${TEST_ROLE}'")
echo "${ROLE_PATH}" | grep -q "search_path=${TEST_ROLE}, public" || fail "ALTER ROLE search_path not applied (got: ${ROLE_PATH})"
pass "ALTER ROLE search_path = ${TEST_ROLE}, public"

# --- 4. Idempotency: second call must succeed and return the same role name ---
step "Second call (idempotency): same UUID"
RESULT2=$(psql -tA -c "SET ROLE lecrm_provisioner; SELECT core.lecrm_provision_workspace('${TEST_UUID}'::uuid);" | tail -n1)
[[ "${RESULT2}" == "${TEST_ROLE}" ]] || fail "idempotent re-call returned ${RESULT2}, expected ${TEST_ROLE}"
pass "idempotent re-call succeeded"

# --- 5. Connect AS the workspace role; confirm search_path inheritance ---
step "Connecting AS ${TEST_ROLE} and checking SHOW search_path"
WORKSPACE_PATH=$(PGUSER="${TEST_ROLE}" PGPASSWORD="" psql -tAc "SHOW search_path;" 2>/dev/null || true)
# trust auth means no password needed; this exercises the role-inherited search_path
[[ -n "${WORKSPACE_PATH}" ]] || fail "could not connect as ${TEST_ROLE}"
echo "${WORKSPACE_PATH}" | tr -d ' ' | grep -q "^\"${TEST_ROLE}\",public$\|^${TEST_ROLE},public$" \
  || fail "SHOW search_path returned '${WORKSPACE_PATH}', expected '${TEST_ROLE}, public'"
pass "SHOW search_path = ${WORKSPACE_PATH}"

# --- 6. Lateral-expansion mitigation: workspace role lacks CREATE on public ---
step "Lateral-expansion mitigation: workspace role lacks CREATE on schema public"
HAS_CREATE=$(psql -tAc "SELECT has_schema_privilege('${TEST_ROLE}', 'public', 'CREATE');")
[[ "${HAS_CREATE}" == "f" ]] || fail "workspace role unexpectedly has CREATE on public (got: ${HAS_CREATE})"
pass "REVOKE CREATE ON SCHEMA public confirmed"

# --- 7. Second distinct UUID provisions cleanly (no cross-tenant collision) ---
step "Sanity: provisioning a second distinct workspace"
SECOND_UUID="bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
SECOND_ROLE="workspace_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
RESULT3=$(psql -tA -c "SET ROLE lecrm_provisioner; SELECT core.lecrm_provision_workspace('${SECOND_UUID}'::uuid);" | tail -n1)
[[ "${RESULT3}" == "${SECOND_ROLE}" ]] || fail "second tenant returned ${RESULT3}, expected ${SECOND_ROLE}"
pass "second tenant provisioned (role=${RESULT3})"

# --- 8. Identity registry: (issuer, sub) uniqueness ---
step "Identity registry: (issuer, sub) uniqueness from 0002_identity.sql"

# Run as lecrm_provisioner without an inline SET (the command tag pollutes
# tuples-only output when followed by INSERT...RETURNING).
export PGOPTIONS="-c role=lecrm_provisioner"

UUID_RE='[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'

USER_A_ID=$(psql -tA -c "INSERT INTO core.users (issuer, subject, email) VALUES ('https://auth.lecrm.test/application/o/lecrm/', 'auth-uuid-1', 'guillaume@gbconsult.example') RETURNING id;" | grep -E "^${UUID_RE}\$" | head -n1)
[[ -n "${USER_A_ID}" ]] || fail "INSERT into core.users returned no id"
pass "Authentik user inserted (id=${USER_A_ID})"

# Same (issuer, sub) must error with unique_violation (23505).
if psql -tA -v ON_ERROR_STOP=1 -c "INSERT INTO core.users (issuer, subject, email) VALUES ('https://auth.lecrm.test/application/o/lecrm/', 'auth-uuid-1', 'dup@example.com');" >/dev/null 2>&1; then
  fail "duplicate (issuer, subject) should have failed"
fi
pass "duplicate (issuer, subject) rejected"

# Same subject, different issuer is a DIFFERENT user (key motivation of
# the tuple: v0→v1 IDP migration is a mapping table, not a destructive
# rewrite — ADR-009 §7.1).
USER_B_ID=$(psql -tA -c "INSERT INTO core.users (issuer, subject, email) VALUES ('https://zitadel.cloud/v2', 'auth-uuid-1', 'guillaume@gbconsult.example') RETURNING id;" | grep -E "^${UUID_RE}\$" | head -n1)
[[ -n "${USER_B_ID}" ]] || fail "second issuer with same subject failed"
[[ "${USER_B_ID}" != "${USER_A_ID}" ]] || fail "same id returned for different issuer"
pass "different issuer, same subject = distinct user (id=${USER_B_ID})"

# Workspace membership FK exercises the cross-table contract.
psql -tA -v ON_ERROR_STOP=1 -c "INSERT INTO core.workspaces (id, slug, role_name) VALUES ('${TEST_UUID}'::uuid, 'acme', '${TEST_ROLE}') ON CONFLICT (id) DO NOTHING; INSERT INTO core.workspace_members (workspace_id, user_id, role) VALUES ('${TEST_UUID}'::uuid, '${USER_A_ID}'::uuid, 'owner');" >/dev/null
pass "workspace membership row created"

unset PGOPTIONS

printf '\n\033[1;32mALL CHECKS PASSED\033[0m on Postgres %s\n' "$(${PGBIN}/postgres --version | awk '{print $3}')"
