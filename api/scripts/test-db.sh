#!/usr/bin/env bash
# Starts a throwaway local PostgreSQL for tests on port 54329 (trust auth).
# Usage: scripts/test-db.sh start|stop|status
# Then: export TEST_DATABASE_URL=postgres://postgres@localhost:54329/postgres?sslmode=disable
set -euo pipefail
PGBIN="${PGBIN:-$(ls -d /usr/lib/postgresql/*/bin 2>/dev/null | sort -V | tail -1)}"
DATA="${PGDATA_DIR:-/tmp/mocpos-pg}"
PORT="${PGPORT:-54329}"
run() { if [ "$(id -u)" = "0" ]; then runuser -u postgres -- "$@"; else "$@"; fi; }
case "${1:-start}" in
  start)
    if [ ! -d "$DATA" ]; then
      mkdir -p "$DATA"; [ "$(id -u)" = "0" ] && chown postgres:postgres "$DATA"
      run "$PGBIN/initdb" -D "$DATA" -U postgres --auth=trust -E UTF8 --locale=C.UTF-8 >/dev/null
      echo "fsync = off" >> "$DATA/postgresql.conf"
      echo "synchronous_commit = off" >> "$DATA/postgresql.conf"
      echo "full_page_writes = off" >> "$DATA/postgresql.conf"
      echo "max_connections = 300" >> "$DATA/postgresql.conf"
    fi
    if ! run "$PGBIN/pg_ctl" -D "$DATA" status >/dev/null 2>&1; then
      run "$PGBIN/pg_ctl" -D "$DATA" -o "-p $PORT -k /tmp" -l "$DATA/server.log" -w start >/dev/null
    fi
    echo "postgres://postgres@localhost:$PORT/postgres?sslmode=disable"
    ;;
  stop) run "$PGBIN/pg_ctl" -D "$DATA" -m fast stop ;;
  status) run "$PGBIN/pg_ctl" -D "$DATA" status ;;
esac
