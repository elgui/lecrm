#!/usr/bin/env bash
# ops/scripts/glockapps-preflight.sh — MANUAL GlockApps content pre-flight for a
# sequence template (ADR-004 rev 2 §8 / §Q2, ADR-003 §Mitigations item 2).
#
# WHY THIS IS MANUAL
# ------------------
# ADR-004 rev 2 §Q2 leaves the GlockApps integration tier OPEN. For v1 the
# default is MANUAL: an operator runs an inbox-placement / spam-score test in the
# GlockApps web UI and records the resulting 0–10 score. Automated triggering
# from the engine needs GlockApps' API tier, which has not been costed/approved —
# so this script makes NO network calls to GlockApps (or anywhere). It is a
# runbook wrapper plus a cheap LOCAL content lint to catch obvious problems
# before you spend a GlockApps test credit; it is NOT a substitute for the real
# GlockApps inbox-placement test.
#
# The recorded score feeds the activation gate in
#   apps/api/internal/sequences/glockapps.go  (CheckTemplateActivation)
# A score < 7/10 BLOCKS template activation unless an admin explicitly overrides.
#
# Usage:
#     ops/scripts/glockapps-preflight.sh <template-file> [--score N] [--override]
#
#   <template-file>   Path to the rendered template body (text or HTML) to lint.
#   --score N         The 0–10 score you obtained from the GlockApps web UI.
#                     When given, the script reports the activation verdict.
#   --override        Acknowledge an admin override for a sub-threshold score
#                     (only meaningful together with a --score below 7).
#
# Exit status:
#   0  content lint clean AND (no --score given OR score activates / overridden)
#   1  usage error / unreadable template
#   2  --score below the 7/10 minimum and NOT overridden (activation blocked)
#
# References:
#   docs/adr/ADR-004-rev2-sequences-architecture.md §8, §Q2
#   docs/adr/ADR-003-email-provider-brevo.md §Mitigations
#   apps/api/internal/sequences/glockapps.go — the enforced gate

set -euo pipefail

MIN_SCORE=7
MAX_SCORE=10
GLOCKAPPS_URL="https://glockapps.com/spam-testing/"

usage() {
  cat <<EOF
Usage: ops/scripts/glockapps-preflight.sh <template-file> [--score N] [--override]

  <template-file>   Rendered template body (text/HTML) to lint locally.
  --score N         0–${MAX_SCORE} score from the GlockApps web UI (${GLOCKAPPS_URL}).
  --override        Acknowledge an admin override for a score < ${MIN_SCORE}.

This script does NOT call any GlockApps API (manual tier, ADR-004 rev 2 §Q2).
EOF
}

# --- args -------------------------------------------------------------------
TEMPLATE=""
SCORE=""
OVERRIDE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --score)    SCORE="${2:-}"; shift 2 ;;
    --override) OVERRIDE=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    -*)         echo "error: unknown flag $1" >&2; usage >&2; exit 1 ;;
    *)          TEMPLATE="$1"; shift ;;
  esac
done

if [[ -z "$TEMPLATE" ]]; then
  echo "error: a <template-file> is required" >&2
  usage >&2
  exit 1
fi
if [[ ! -r "$TEMPLATE" ]]; then
  echo "error: cannot read template file: $TEMPLATE" >&2
  exit 1
fi

# --- step 1: the manual GlockApps workflow ---------------------------------
cat <<EOF
GlockApps pre-flight (MANUAL — ADR-004 rev 2 §Q2)
=================================================
Template : $TEMPLATE

1. Open the GlockApps Spam Testing tool:
     ${GLOCKAPPS_URL}
2. Start a new test; paste the rendered subject + body of this template and
   send to the GlockApps seed-list address it gives you (use the leCRM Brevo
   sender so DKIM/SPF/DMARC are exercised exactly as production).
3. Read the Inbox-Placement / Spam-score result and convert it to a 0–${MAX_SCORE}
   score (10 = inbox everywhere, 0 = spam/blocked everywhere).
4. Re-run this script with --score N to record the verdict, e.g.
     ops/scripts/glockapps-preflight.sh "$TEMPLATE" --score 8

EOF

# --- step 2: local content lint (pre-GlockApps sanity, NOT a substitute) ----
echo "Local content lint (heuristics only — NOT a GlockApps result)"
echo "-------------------------------------------------------------"
warnings=0
warn() { echo "  [warn] $1"; warnings=$((warnings + 1)); }

# Common spam-trigger phrases (case-insensitive). Not exhaustive; indicative.
SPAM_WORDS='free money|act now|limited time|click here|100% free|risk[- ]free|guarantee|winner|congratulations|viagra|cash bonus|no cost|order now|buy now|cheap|earn \$'
SPAM_HITS=$({ grep -E -i -o -- "$SPAM_WORDS" "$TEMPLATE" || true; } | sort -u | paste -sd, -)
if [[ -n "$SPAM_HITS" ]]; then
  warn "spam-trigger phrases present: $SPAM_HITS"
fi

# Unsubscribe affordance (List-Unsubscribe is added at send time, but body link
# is still best practice for cold outreach).
if ! grep -E -i -q -- 'unsubscribe|se désinscrire|désabonner|opt[- ]out' "$TEMPLATE"; then
  warn "no visible unsubscribe / opt-out language found"
fi

# Excessive links raise spam scores. -o prints one match per line; grep exits 1
# on no match, so `|| true` keeps the count at 0 under `set -e`/`pipefail`.
LINK_COUNT=$({ grep -E -o -i -- 'https?://' "$TEMPLATE" || true; } | wc -l | tr -d ' ')
if [[ "${LINK_COUNT:-0}" -gt 5 ]]; then
  warn "high link count ($LINK_COUNT); keep cold-outreach links minimal"
fi

# ALL-CAPS shouting in any line is a classic spam signal.
if grep -E -q -- '[[:upper:]]{12,}' "$TEMPLATE"; then
  warn "long ALL-CAPS run detected; avoid shouting"
fi

if [[ "$warnings" -eq 0 ]]; then
  echo "  clean — no local heuristics tripped (still run the GlockApps test)"
fi
echo

# --- step 3: score verdict (mirrors glockapps.go CheckTemplateActivation) ----
if [[ -z "$SCORE" ]]; then
  echo "No --score given: record the GlockApps score, then re-run with --score N."
  exit 0
fi

if ! [[ "$SCORE" =~ ^[0-9]+$ ]] || (( SCORE < 0 || SCORE > MAX_SCORE )); then
  echo "error: --score must be an integer in [0, ${MAX_SCORE}] (got: $SCORE)" >&2
  exit 1
fi

echo "Activation verdict (min ${MIN_SCORE}/${MAX_SCORE})"
echo "------------------------------------"
if (( SCORE >= MIN_SCORE )); then
  echo "  ALLOWED — score ${SCORE}/${MAX_SCORE} meets the ${MIN_SCORE}/${MAX_SCORE} minimum."
  exit 0
fi

if (( OVERRIDE == 1 )); then
  echo "  ALLOWED (ADMIN OVERRIDE) — score ${SCORE}/${MAX_SCORE} is below the ${MIN_SCORE}/${MAX_SCORE} minimum."
  echo "  Record this override on the activation audit trail (sequences.template.activation_override)."
  exit 0
fi

echo "  BLOCKED — score ${SCORE}/${MAX_SCORE} is below the ${MIN_SCORE}/${MAX_SCORE} minimum."
echo "  Improve the content and re-test, or re-run with --override for an explicit admin override."
exit 2
