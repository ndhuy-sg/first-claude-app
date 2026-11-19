#!/usr/bin/env bash
# Test case N-02: restore the latest backup from R2 into a SCRATCH cluster and compare row counts
# with the live database. The live database is never touched (read-only queries only).
# Run monthly (docs/DEPLOY.md):
#
#   docker compose run --rm backup restore-test.sh            # use the latest base backup
#   docker compose run --rm backup restore-test.sh --fresh    # take a new base backup first
#
# Steps: force a WAL switch on live and wait until it is archived → count rows on live
# → fetch the latest base backup into a temp dir → replay archived WAL (recovery) on a private
# port → count rows on the restored copy → count rows on live again → compare → clean up.
set -euo pipefail
. /usr/local/bin/walg-env.sh

SCRATCH="${RESTORE_DIR:-/tmp/mocpos-restore-test}"
PORT="${RESTORE_PORT:-55432}"
DB="${PGDATABASE:-mocpos}"
TIMEOUT="${RESTORE_TIMEOUT_SECONDS:-1800}"
WORK="$(mktemp -d)"
RESTORED_URL="postgresql:///$DB?host=/tmp&port=$PORT&user=${PGUSER:-postgres}"

cleanup() {
  pg_ctl -D "$SCRATCH" -m immediate stop >/dev/null 2>&1 || true
  rm -rf "$SCRATCH" "$WORK"
}
trap cleanup EXIT

[ "${1:-}" = "--fresh" ] && base-backup.sh

log "forcing a WAL switch on live and waiting for it to be archived"
seg=$(psql -X -A -t -v ON_ERROR_STOP=1 -c "SELECT pg_walfile_name(pg_switch_wal())")
for _ in $(seq 1 120); do
  last=$(psql -X -A -t -c "SELECT coalesce(last_archived_wal, '') FROM pg_stat_archiver")
  if [[ "$last" > "$seg" || "$last" == "$seg" ]]; then break; fi
  sleep 1
done
[[ "$last" > "$seg" || "$last" == "$seg" ]] || { log "WAL $seg was not archived within 120 s (is BACKUP_ENABLED=true on postgres?)"; exit 1; }

log "row counts on live (before)"
row-counts.sh > "$WORK/live-before.txt"

log "fetching the latest base backup into $SCRATCH"
rm -rf "$SCRATCH"; mkdir -p "$SCRATCH"; chmod 0700 "$SCRATCH"
wal-g backup-fetch "$SCRATCH" LATEST
touch "$SCRATCH/recovery.signal"

log "replaying WAL on a private instance (port $PORT)"
pg_ctl -D "$SCRATCH" -l "$WORK/restore.log" -w -t "$TIMEOUT" start -o "\
  -c port=$PORT -c listen_addresses='' -c unix_socket_directories=/tmp \
  -c archive_mode=off -c shared_buffers=128MB -c hot_standby=on \
  -c restore_command='/usr/local/bin/wal-fetch.sh %f %p' -c recovery_target_action=promote" \
  || { tail -n 50 "$WORK/restore.log"; exit 1; }

deadline=$(( $(date +%s) + TIMEOUT ))
until [ "$(psql "$RESTORED_URL" -X -A -t -c 'SELECT pg_is_in_recovery()' 2>/dev/null)" = "f" ]; do
  [ "$(date +%s)" -lt "$deadline" ] || { log "recovery did not finish in ${TIMEOUT}s"; tail -n 50 "$WORK/restore.log"; exit 1; }
  sleep 2
done

log "row counts on the restored copy"
row-counts.sh "$RESTORED_URL" > "$WORK/restored.txt"
log "row counts on live (after)"
row-counts.sh > "$WORK/live-after.txt"

compare-counts.sh "$WORK/live-before.txt" "$WORK/restored.txt" "$WORK/live-after.txt"
