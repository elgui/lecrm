#!/usr/bin/env bash
# embed-spa.sh — build the React SPA and stage it for go:embed.
#
# Produces apps/web/dist via the web build, then mirrors it into
# apps/api/internal/spa/dist so `//go:embed all:dist` pulls the freshly
# built assets into the lecrm-api binary (ADR-009 §5.1, Sprint 9).
#
# Usage:
#   scripts/embed-spa.sh                 # build web, then stage
#   SKIP_WEB_BUILD=1 scripts/embed-spa.sh  # stage an already-built dist
#
# The package manager defaults to bun (the repo lockfile is bun.lock);
# override with PKG=npm / PKG=pnpm if needed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIST="$ROOT/apps/web/dist"
EMBED_DIST="$ROOT/apps/api/internal/spa/dist"
PKG="${PKG:-bun}"

if [[ "${SKIP_WEB_BUILD:-0}" != "1" ]]; then
  echo "==> building SPA (apps/web) with $PKG"
  # Canonical build is `vite build`. On hosts with a tight RLIMIT_AS the
  # bundler's WebAssembly step can't reserve its address space; fall back to
  # the esbuild build (native binary + Tailwind CLI, no WebAssembly).
  if ! ( cd "$ROOT/apps/web" && "$PKG" run build ); then
    echo "==> vite build failed; falling back to esbuild build" >&2
    ( cd "$ROOT/apps/web" && node scripts/build-esbuild.mjs )
  fi
fi

if [[ ! -f "$WEB_DIST/index.html" ]]; then
  echo "error: $WEB_DIST/index.html not found — did the web build succeed?" >&2
  exit 1
fi

echo "==> staging $WEB_DIST -> $EMBED_DIST"
# Wipe everything except the tracked .gitkeep, then copy the fresh build.
find "$EMBED_DIST" -mindepth 1 ! -name '.gitkeep' -delete
cp -R "$WEB_DIST"/. "$EMBED_DIST"/

echo "==> staged $(find "$EMBED_DIST" -type f | wc -l | tr -d ' ') files"
echo "Now build the binary: (cd apps/api && go build -o lecrm-api ./cmd/lecrm-api)"
