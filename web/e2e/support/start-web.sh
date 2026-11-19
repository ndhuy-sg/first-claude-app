#!/usr/bin/env bash
# Starts Next.js on :3000 for Playwright.
#   E2E_WEB_MODE=dev  (default locally)  → next dev
#   E2E_WEB_MODE=prod (CI, after `npm run build`) → the standalone server, like production
# /api/* is rewritten to API_ORIGIN (default http://localhost:8080, baked in at build time for prod).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
PORT="${E2E_WEB_PORT:-3000}"

if [ "${E2E_WEB_MODE:-dev}" = "prod" ]; then
  [ -f .next/standalone/server.js ] || { echo "run 'npm run build' first" >&2; exit 1; }
  rm -rf .next/standalone/.next/static .next/standalone/public
  cp -r .next/static .next/standalone/.next/static
  [ -d public ] && cp -r public .next/standalone/public
  PORT="$PORT" HOSTNAME=localhost exec node .next/standalone/server.js
fi
exec npx next dev -p "$PORT"
