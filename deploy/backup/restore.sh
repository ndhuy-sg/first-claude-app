#!/usr/bin/env bash
# DISASTER RECOVERY: restore the database from R2 into the (empty) postgres data volume.
# Used when the VM died or when moving to a new server (docs/DEPLOY.md, "Khôi phục").
#
#   docker compose stop api worker web postgres
#   docker compose run --rm --no-deps backup restore.sh                       # latest state
#   RECOVERY_TARGET_TIME="2026-11-19 14:30:00+07" docker compose run --rm --no-deps \
#       -e RECOVERY_TARGET_TIME backup restore.sh                            # point in time
#   docker compose up -d
#
# It refuses to overwrite a data directory that already holds a cluster unless FORCE=yes
# (then the old data is moved aside to $PGDATA.old-<timestamp>, never deleted).
set -euo pipefail
. /usr/local/bin/walg-env.sh
: "${PGDATA:=/var/lib/postgresql/data}"
BACKUP_NAME="${BACKUP_NAME:-LATEST}"

if pg_isready -q 2>/dev/null; then
  log "postgres is still running. Stop it first: docker compose stop postgres"; exit 1
fi

if [ -s "$PGDATA/PG_VERSION" ]; then
  if [ "${FORCE:-}" != "yes" ]; then
    log "$PGDATA already contains a database. Re-run with FORCE=yes to move it aside and restore."; exit 1
  fi
  aside="$PGDATA.old-$(date +%Y%m%d%H%M%S)"
  log "moving the current data directory contents to $aside"
  mkdir -p "$aside"
  find "$PGDATA" -mindepth 1 -maxdepth 1 -exec mv -t "$aside" {} +
fi

log "available backups:"; wal-g backup-list
log "fetching $BACKUP_NAME into $PGDATA"
wal-g backup-fetch "$PGDATA" "$BACKUP_NAME"
chmod 0700 "$PGDATA"

# Recovery settings are appended to postgresql.auto.conf; PostgreSQL applies them on the next start,
# replays WAL from R2 and then promotes itself to a normal primary.
{
  echo "# --- added by restore.sh $(date -Iseconds) ---"
  echo "restore_command = '/usr/local/bin/wal-fetch.sh %f %p'"
  echo "recovery_target_action = 'promote'"
  if [ -n "${RECOVERY_TARGET_TIME:-}" ]; then echo "recovery_target_time = '$RECOVERY_TARGET_TIME'"; fi
} >> "$PGDATA/postgresql.auto.conf"
touch "$PGDATA/recovery.signal"
# Run as root (docker compose run --user root …)? Hand the files back to the postgres user.
if [ "$(id -u)" = "0" ]; then chown -R postgres:postgres "$PGDATA"; fi

log "done. Start the stack with: docker compose up -d   (postgres replays WAL first; watch: docker compose logs -f postgres)"
# The restore_command line may stay in postgresql.auto.conf: it is only used during recovery.
# recovery_target_time must not stay (a later restore would stop there), so it is only written when asked.
log "if you used RECOVERY_TARGET_TIME, delete that line from $PGDATA/postgresql.auto.conf after the start"
