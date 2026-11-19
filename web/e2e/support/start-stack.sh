#!/usr/bin/env bash
# Starts the Go API for Playwright on :8080 against a FRESH migrated + seeded database.
# Used by web/playwright.config.ts (webServer). Can also be run by hand:
#
#   api/scripts/test-db.sh start
#   TEST_DATABASE_URL=postgres://postgres@localhost:54329/postgres?sslmode=disable web/e2e/support/start-stack.sh
#
# Env:
#   TEST_DATABASE_URL  admin connection to a PostgreSQL server (default: the local test-db.sh server)
#   E2E_DB_NAME        database to (re)create, default mocpos_e2e
#   MOCPOS_BIN         prebuilt mocpos binary (CI builds it once); otherwise built from api/
#   E2E_API_PORT       default 8080
#   E2E_WEB_ORIGIN     default http://localhost:3000 (cookie origin / ALLOWED_ORIGINS)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
ADMIN_URL="${TEST_DATABASE_URL:-postgres://postgres@localhost:54329/postgres?sslmode=disable}"
DB_NAME="${E2E_DB_NAME:-mocpos_e2e}"
PORT="${E2E_API_PORT:-8080}"
ORIGIN="${E2E_WEB_ORIGIN:-http://localhost:3000}"

[[ "$DB_NAME" =~ ^[a-z_][a-z0-9_]*$ ]] || { echo "E2E_DB_NAME must be a plain identifier" >&2; exit 1; }
command -v psql >/dev/null || { echo "psql is required (postgresql-client)" >&2; exit 1; }

# Same server and params, different database: replace the path of the URL.
DB_URL="$(printf '%s' "$ADMIN_URL" | sed -E "s#^([a-z]+://[^/]+)/[^?]*#\1/${DB_NAME}#")"
[[ "$DB_URL" == *"/${DB_NAME}"* ]] || DB_URL="${ADMIN_URL%%\?*}/${DB_NAME}"

BIN="${MOCPOS_BIN:-}"
if [ -z "$BIN" ]; then
  BIN="$ROOT/api/bin/mocpos"
  echo "[e2e] building mocpos → $BIN"
  (cd "$ROOT/api" && go build -o "$BIN" ./cmd/mocpos)
fi

echo "[e2e] recreating database $DB_NAME"
psql "$ADMIN_URL" -X -q -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE IF EXISTS \"$DB_NAME\" WITH (FORCE)" \
  -c "CREATE DATABASE \"$DB_NAME\""

export DATABASE_URL="$DB_URL"
echo "[e2e] migrate"; "$BIN" migrate
echo "[e2e] seed";    "$BIN" seed

export HTTP_ADDR=":$PORT" COOKIE_SECURE=false ALLOWED_ORIGINS="$ORIGIN" PUBLIC_BASE_URL="$ORIGIN"
# Fake tax authority run by the F-03 spec (only used when settings enable submission).
export TAX_GATEWAY_URL="${E2E_TAX_GATEWAY_URL:-http://localhost:${E2E_TAX_PORT:-18090}}"

pids=()
cleanup() { kill "${pids[@]}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

if [ "${E2E_START_WORKER:-1}" = "1" ]; then
  echo "[e2e] worker"
  "$BIN" worker & pids+=($!)
fi
echo "[e2e] serve on :$PORT (origin $ORIGIN)"
"$BIN" serve & pids+=($!)
# Exit (and stop the other process) as soon as either of them stops.
wait -n

