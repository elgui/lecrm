#!/usr/bin/env bash
# ops/provision-client.sh — render an encrypted client manifest into a
# Compose-loadable .env file on the deploy host.
#
# Usage:
#     ops/provision-client.sh <workspace-slug> [--env-out <path>]
#
# Reads :  secrets/clients/<slug>/secrets.enc.yaml
# Writes:  deploy/.env.<slug>   (mode 0600, gitignored)
#
# The .env file is consumed by:
#     docker compose --env-file deploy/.env.<slug> \
#                    -f deploy/compose/postgres.yml \
#                    -f deploy/compose/authentik.yml \
#                    up -d
#
# Per ADR-007 §2 the script never holds plaintext in a regular file
# any longer than necessary — decryption goes through a process
# substitution and the rendered env file is written with restrictive
# mode. The age private key is sourced from $SOPS_AGE_KEY_FILE
# (default ~/.config/sops/age/keys.txt) on the operator workstation,
# or from a YubiKey via age-plugin-yubikey.
#
# Naming convention (yaml key → env var):
#     db_role_password             → LECRM_DB_ROLE_PASSWORD
#     oauth_gmail_client_secret    → LECRM_OAUTH_GMAIL_CLIENT_SECRET
#     jwt_signing_key              → LECRM_JWT_SIGNING_KEY
#     brevo_api_key                → LECRM_BREVO_API_KEY
#     brevo_webhook_secret         → LECRM_BREVO_WEBHOOK_SECRET
#
# Operator-only fields from secrets/operator/secrets.enc.yaml are NOT
# merged here — they live in their own rendered file (see
# `--also-operator` flag) and are loaded by the deploy host's systemd
# unit, not by Compose.

set -euo pipefail

usage() {
  cat <<EOF
Usage: ops/provision-client.sh <workspace-slug> [options]

Options:
  --env-out PATH        Write the rendered env to PATH (default deploy/.env.<slug>)
  --also-operator       Additionally render secrets/operator/secrets.enc.yaml
                        to deploy/.env.operator (Tier-0 secrets; the deploy
                        host's systemd EnvironmentFile, NOT a Compose env_file).
  --dry-run             Print the rendered env file to stdout instead of writing.
  -h | --help           This message.
EOF
}

if [[ $# -lt 1 ]]; then usage; exit 2; fi

SLUG=""
ENV_OUT=""
ALSO_OPERATOR="false"
DRY_RUN="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)        usage; exit 0 ;;
    --env-out)        ENV_OUT="${2:-}"; shift 2 ;;
    --also-operator)  ALSO_OPERATOR="true"; shift ;;
    --dry-run)        DRY_RUN="true"; shift ;;
    --)               shift; break ;;
    -*)               echo "unknown flag: $1" >&2; usage; exit 2 ;;
    *)
      if [[ -z "${SLUG}" ]]; then SLUG="$1"; shift
      else echo "unexpected arg: $1" >&2; usage; exit 2
      fi ;;
  esac
done

if [[ -z "${SLUG}" ]]; then echo "missing <workspace-slug>" >&2; usage; exit 2; fi
if [[ ! "${SLUG}" =~ ^[a-z0-9][a-z0-9-]*[a-z0-9]$ ]]; then
  echo "error: slug must be lowercase kebab-case (got '${SLUG}')" >&2
  exit 2
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "${REPO_ROOT}"

for tool in sops python3; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "error: ${tool} not found on PATH" >&2
    exit 3
  fi
done

CLIENT_MANIFEST="secrets/clients/${SLUG}/secrets.enc.yaml"
if [[ ! -f "${CLIENT_MANIFEST}" ]]; then
  echo "error: ${CLIENT_MANIFEST} does not exist — create it first per ops/secrets/README.md" >&2
  exit 4
fi

ENV_OUT="${ENV_OUT:-deploy/.env.${SLUG}}"

# Render a YAML file to KEY=value lines. Keys are upper-snake-cased and
# prefixed with LECRM_; values are single-quoted with embedded single
# quotes escaped (POSIX shell rules). Booleans and numerics are emitted
# verbatim.
render_yaml_to_env() {
  local prefix="$1"; shift
  sops --config ops/secrets/.sops.yaml --decrypt "$@" \
    | python3 - "${prefix}" <<'PY'
import sys, yaml
prefix = sys.argv[1]
doc = yaml.safe_load(sys.stdin.read()) or {}
if not isinstance(doc, dict):
    print("error: manifest root must be a mapping", file=sys.stderr)
    sys.exit(5)
for k, v in doc.items():
    if v is None or v == "":
        # Skip empty fields — they are placeholders in a partially-filled manifest.
        continue
    if isinstance(v, bool):
        rendered = "true" if v else "false"
    elif isinstance(v, (int, float)):
        rendered = str(v)
    else:
        s = str(v).replace("'", "'\"'\"'")
        rendered = f"'{s}'"
    print(f"{prefix}{k.upper()}={rendered}")
PY
}

write_atomic() {
  local target="$1"
  local tmp
  tmp="$(mktemp "${target}.XXXXXX")"
  chmod 600 "${tmp}"
  cat >"${tmp}"
  mv "${tmp}" "${target}"
  chmod 600 "${target}"
}

# --- Client manifest -------------------------------------------------
if [[ "${DRY_RUN}" == "true" ]]; then
  echo "# rendered from ${CLIENT_MANIFEST}"
  render_yaml_to_env "LECRM_" "${CLIENT_MANIFEST}"
else
  rendered_client="$(render_yaml_to_env "LECRM_" "${CLIENT_MANIFEST}")"
  {
    echo "# rendered ${CLIENT_MANIFEST} → ${ENV_OUT}"
    echo "# do NOT commit; sourced by docker compose --env-file at deploy time"
    echo "${rendered_client}"
  } | write_atomic "${ENV_OUT}"
  echo "wrote ${ENV_OUT} (mode 0600)"
fi

# --- Operator manifest (opt-in) --------------------------------------
if [[ "${ALSO_OPERATOR}" == "true" ]]; then
  OPERATOR_MANIFEST="secrets/operator/secrets.enc.yaml"
  if [[ ! -f "${OPERATOR_MANIFEST}" ]]; then
    echo "error: ${OPERATOR_MANIFEST} does not exist — bootstrap it first" >&2
    exit 6
  fi
  OPERATOR_ENV="deploy/.env.operator"
  if [[ "${DRY_RUN}" == "true" ]]; then
    echo
    echo "# rendered from ${OPERATOR_MANIFEST}"
    render_yaml_to_env "" "${OPERATOR_MANIFEST}"
  else
    rendered_op="$(render_yaml_to_env "" "${OPERATOR_MANIFEST}")"
    {
      echo "# rendered ${OPERATOR_MANIFEST} → ${OPERATOR_ENV}"
      echo "# Tier-0 platform secrets; load via systemd EnvironmentFile, not compose env_file"
      echo "${rendered_op}"
    } | write_atomic "${OPERATOR_ENV}"
    echo "wrote ${OPERATOR_ENV} (mode 0600)"
  fi
fi
