#!/usr/bin/env bash
# ops/secrets/bootstrap.sh — one-shot age + SOPS bootstrap for leCRM v0.
#
# Generates Guillaume's age keypair, wires the public key into the
# SOPS policy, and prints the YubiKey + Bitwarden custody checklist
# that only a human can complete. Idempotent: refuses to clobber an
# existing leCRM-tagged key.
#
# Invoke from the repository root:
#
#     ops/secrets/bootstrap.sh
#
# See ops/secrets/README.md for the full procedure.

set -euo pipefail

# --- Resolve paths ---------------------------------------------------
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "${REPO_ROOT}" ]]; then
  echo "error: run this script from inside the leCRM git repo" >&2
  exit 2
fi
cd "${REPO_ROOT}"

SOPS_POLICY="ops/secrets/.sops.yaml"
PUBKEY_FILE="ops/secrets/recipients/guillaume.age.pub"
PRIVKEY_DIR="${HOME}/.config/sops/age"
PRIVKEY_FILE="${PRIVKEY_DIR}/keys.txt"
LECRM_KEY_TAG="# leCRM v0 — Guillaume"

# --- Tool presence ---------------------------------------------------
for tool in age age-keygen sops; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "error: ${tool} not found on PATH (see ops/secrets/README.md)" >&2
    exit 2
  fi
done

# --- Idempotency guard -----------------------------------------------
if [[ -f "${PRIVKEY_FILE}" ]] && grep -qF "${LECRM_KEY_TAG}" "${PRIVKEY_FILE}"; then
  echo "info: leCRM age key already present in ${PRIVKEY_FILE} — nothing to do"
  echo "       (delete the tagged block manually if you really want to re-key)"
  exit 0
fi
if [[ -f "${PUBKEY_FILE}" ]] && [[ -s "${PUBKEY_FILE}" ]]; then
  echo "error: ${PUBKEY_FILE} already exists and is non-empty" >&2
  echo "       refusing to overwrite a recipient on file" >&2
  exit 1
fi

# --- Generate keypair ------------------------------------------------
mkdir -p "${PRIVKEY_DIR}"
chmod 700 "${PRIVKEY_DIR}"

TMP_KEYFILE="$(mktemp)"
trap 'rm -f "${TMP_KEYFILE}"' EXIT

age-keygen -o "${TMP_KEYFILE}" >/dev/null
chmod 600 "${TMP_KEYFILE}"

# Extract the public-key line (age-keygen writes it as a `# public key: …` comment).
PUBKEY="$(grep -oE 'age1[0-9a-z]+' "${TMP_KEYFILE}" | head -n1)"
if [[ -z "${PUBKEY}" ]]; then
  echo "error: could not extract age public key from generated file" >&2
  exit 3
fi

# --- Append to ~/.config/sops/age/keys.txt with a tag ----------------
{
  echo
  echo "${LECRM_KEY_TAG}"
  cat "${TMP_KEYFILE}"
} >>"${PRIVKEY_FILE}"
chmod 600 "${PRIVKEY_FILE}"

# --- Write the public-key recipient file -----------------------------
mkdir -p "$(dirname "${PUBKEY_FILE}")"
printf '%s\n' "${PUBKEY}" >"${PUBKEY_FILE}"

# --- Patch .sops.yaml ------------------------------------------------
if [[ ! -f "${SOPS_POLICY}" ]]; then
  echo "error: ${SOPS_POLICY} not found (was the repo scaffolding committed?)" >&2
  exit 4
fi
if ! grep -qF "REPLACE_WITH_AGE_PUBLIC_KEY" "${SOPS_POLICY}"; then
  echo "warning: no REPLACE_WITH_AGE_PUBLIC_KEY token in ${SOPS_POLICY}"
  echo "         skipping policy patch (already initialised?)"
else
  # In-place replacement that works on both GNU and BSD sed.
  python3 - "${SOPS_POLICY}" "${PUBKEY}" <<'PY'
import pathlib
import sys
path, pubkey = pathlib.Path(sys.argv[1]), sys.argv[2]
path.write_text(path.read_text().replace("REPLACE_WITH_AGE_PUBLIC_KEY", pubkey))
PY
fi

# --- Final report ----------------------------------------------------
cat <<EOF

age keypair generated and wired into SOPS.

  Public key file : ${PUBKEY_FILE}
  Private key file: ${PRIVKEY_FILE}   (mode 0600; back this up)
  Policy patched  : ${SOPS_POLICY}

NEXT STEPS — manual, per ADR-007 §2 (custody)

  1. YubiKey (primary custody).
     Option A (preferred): re-generate the age key using
       \`age-plugin-yubikey\` so the private key NEVER touches disk:
         age-plugin-yubikey --generate
       Then replace the keys.txt block above with the resulting
       identity file. (If you do this, also overwrite the public key
       in ${PUBKEY_FILE} and re-run \`sops updatekeys\` on every
       existing encrypted manifest.)
     Option B (acceptable v0): encrypt keys.txt with the PGP key
       resident on your YubiKey and keep the ciphertext in 1Password /
       Bitwarden / etc.
  2. Bitwarden (backup custody).
     Paste the full content of ${PRIVKEY_FILE} (every \`AGE-SECRET-KEY-\`
     line plus its tag header) into a Bitwarden secure note tagged
     "leCRM / age / 2026".
  3. Commit the bootstrap output:
       git add ${SOPS_POLICY} ${PUBKEY_FILE}
       git commit -m "ops(secrets): bootstrap age recipient"
  4. Record this bootstrap event in
     ops/runbooks/secret-rotation.md → \`## Bootstrap & rotation log\`.

Done.
EOF
