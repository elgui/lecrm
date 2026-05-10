#!/usr/bin/env bash
# =====================================================================
# leCRM — per-client provisioning script (Phase 1, VPS-per-client)
#
# Renders the docker-compose template + Caddyfile + .env into
# `ops/clients/<client-slug>/` and brings the stack up.
#
# Usage:
#   ./ops/provision-client.sh \
#     --client-slug acme \
#     --subdomain acme.lecrm.fr \
#     --client-name "Acme SARL" \
#     --client-domain acme.fr
#
# Optional overrides (otherwise prompted or generated):
#   --brevo-smtp-login ...        Brevo SMTP login (numeric ID)
#   --brevo-smtp-key ...          Brevo SMTP key (begins with `xkeysmtp`)
#   --oidc-client-id ...          Google Workspace OAuth client ID
#   --oidc-client-secret ...      Google Workspace OAuth client secret
#   --image-tag twenty-X.Y.Z-lecrm.N   Override default leCRM image tag
#
# After provisioning:
#   1. Point DNS A/AAAA for <subdomain> at this VPS's public IP.
#   2. Wait for Caddy to provision the Let's Encrypt cert (~30s).
#   3. Smoke test: curl -I https://<subdomain>/api/version
# =====================================================================

set -euo pipefail

# --- Defaults --------------------------------------------------------
CLIENT_SLUG=""
CLIENT_SUBDOMAIN=""
CLIENT_NAME=""
CLIENT_DOMAIN=""
BREVO_SMTP_LOGIN=""
BREVO_SMTP_KEY=""
OIDC_CLIENT_ID=""
OIDC_CLIENT_SECRET=""
IMAGE_TAG="ghcr.io/elgui/lecrm:twenty-2.2.0-lecrm.0"

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEMPLATE_DIR="$REPO_ROOT/ops/templates"
CLIENTS_DIR="$REPO_ROOT/ops/clients"

# --- Argument parsing ------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --client-slug)         CLIENT_SLUG="$2";       shift 2 ;;
    --subdomain)           CLIENT_SUBDOMAIN="$2";  shift 2 ;;
    --client-name)         CLIENT_NAME="$2";       shift 2 ;;
    --client-domain)       CLIENT_DOMAIN="$2";     shift 2 ;;
    --brevo-smtp-login)    BREVO_SMTP_LOGIN="$2";  shift 2 ;;
    --brevo-smtp-key)      BREVO_SMTP_KEY="$2";    shift 2 ;;
    --oidc-client-id)      OIDC_CLIENT_ID="$2";    shift 2 ;;
    --oidc-client-secret)  OIDC_CLIENT_SECRET="$2"; shift 2 ;;
    --image-tag)           IMAGE_TAG="$2";         shift 2 ;;
    -h|--help)
      sed -n '2,30p' "$0"
      exit 0
      ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

# --- Validation ------------------------------------------------------
require() {
  local var_name="$1"; local human_name="$2"
  if [[ -z "${!var_name}" ]]; then
    echo "ERROR: $human_name is required (set via --$(echo "$human_name" | tr '[:upper:] ' '[:lower:]-'))" >&2
    exit 2
  fi
}

require CLIENT_SLUG "client-slug"
require CLIENT_SUBDOMAIN "subdomain"
require CLIENT_NAME "client-name"
require CLIENT_DOMAIN "client-domain"

# Slug shape: lowercase, alphanumeric + hyphen
if [[ ! "$CLIENT_SLUG" =~ ^[a-z][a-z0-9-]{1,30}$ ]]; then
  echo "ERROR: client-slug must be lowercase alphanumeric+hyphen, 2-31 chars (got: '$CLIENT_SLUG')" >&2
  exit 2
fi

CLIENT_DIR="$CLIENTS_DIR/$CLIENT_SLUG"
if [[ -d "$CLIENT_DIR" ]]; then
  echo "ERROR: $CLIENT_DIR already exists. Refusing to overwrite. Remove manually if intentional." >&2
  exit 2
fi

# --- Secret generation -----------------------------------------------
gen_secret() {
  # 48-byte URL-safe random string
  python3 -c "import secrets; print(secrets.token_urlsafe(48))" 2>/dev/null \
    || openssl rand -base64 48 | tr -d '\n=' | tr '+/' '-_'
}

PG_PASSWORD="$(gen_secret)"
APP_SECRET="$(gen_secret)"
SLUG_UNDERSCORE="${CLIENT_SLUG//-/_}"

# --- Render -----------------------------------------------------------
mkdir -p "$CLIENT_DIR"

# .env
sed \
  -e "s|__SLUG__|$CLIENT_SLUG|g" \
  -e "s|__SUBDOMAIN__|$CLIENT_SUBDOMAIN|g" \
  -e "s|__CLIENT_DOMAIN__|$CLIENT_DOMAIN|g" \
  -e "s|__CLIENT_NAME__|$CLIENT_NAME|g" \
  -e "s|__SLUG_UNDERSCORE__|$SLUG_UNDERSCORE|g" \
  -e "s|__GENERATED_PG_PASSWORD__|$PG_PASSWORD|g" \
  -e "s|__GENERATED_APP_SECRET__|$APP_SECRET|g" \
  -e "s|__BREVO_SMTP_LOGIN__|${BREVO_SMTP_LOGIN:-CHANGE_ME}|g" \
  -e "s|__BREVO_SMTP_KEY__|${BREVO_SMTP_KEY:-CHANGE_ME}|g" \
  -e "s|__OIDC_CLIENT_ID__|${OIDC_CLIENT_ID:-CHANGE_ME}|g" \
  -e "s|__OIDC_CLIENT_SECRET__|${OIDC_CLIENT_SECRET:-CHANGE_ME}|g" \
  "$TEMPLATE_DIR/.env.template" > "$CLIENT_DIR/.env"
chmod 600 "$CLIENT_DIR/.env"

# Override the default image if --image-tag was passed
if [[ "$IMAGE_TAG" != "ghcr.io/elgui/lecrm:twenty-2.2.0-lecrm.0" ]]; then
  sed -i "s|^LECRM_IMAGE=.*|LECRM_IMAGE=$IMAGE_TAG|" "$CLIENT_DIR/.env"
fi

# docker-compose.yml — copy verbatim (the template uses ${VAR} interpolation
# from .env, no sed substitution needed)
cp "$TEMPLATE_DIR/docker-compose.template.yml" "$CLIENT_DIR/docker-compose.yml"

# Caddyfile — substitute the subdomain (Caddy doesn't read .env directly
# without the `envsubst` pattern; cleanest is one-shot rendering).
sed "s|{\$CLIENT_SUBDOMAIN}|$CLIENT_SUBDOMAIN|g" "$TEMPLATE_DIR/Caddyfile.template" > "$CLIENT_DIR/Caddyfile"

# --- Summary ---------------------------------------------------------
echo "Provisioned $CLIENT_DIR"
echo
echo "Next steps:"
echo "  1. Review/adjust placeholders in $CLIENT_DIR/.env (BREVO_*, OIDC_*)."
echo "  2. cd $CLIENT_DIR && docker compose up -d"
echo "  3. Wait ~30s for Caddy to obtain the Let's Encrypt cert."
echo "  4. Smoke: curl -fsS https://$CLIENT_SUBDOMAIN/api/version"
echo
echo "If the smoke check fails, inspect logs with:"
echo "  docker compose -f $CLIENT_DIR/docker-compose.yml logs --tail 100"
