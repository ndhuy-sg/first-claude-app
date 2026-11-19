#!/usr/bin/env bash
# Takes one full (base) backup of the live cluster to R2 and applies the 30-day retention.
# Runs in the "backup" sidecar, which shares the postgres data volume (read access to $PGDATA)
# and connects to the server over the compose network.
#
#   docker compose exec backup base-backup.sh      # on demand
#
# Retention: one full backup per night, keep BACKUP_RETENTION_FULL (default 30) of them, so the
# oldest restorable point is ~30 days back. WAL older than the oldest kept full backup is deleted
# together with it (wal-g "delete retain FULL").
set -euo pipefail
. /usr/local/bin/walg-env.sh

: "${PGDATA:=/var/lib/postgresql/data}"
: "${BACKUP_RETENTION_FULL:=30}"

ping_heartbeat() { # optional dead-man switch, e.g. https://hc-ping.com/<uuid> (healthchecks.io free)
  [ -n "${BACKUP_HEARTBEAT_URL:-}" ] || return 0
  curl -fsS -m 10 --retry 3 -o /dev/null "${BACKUP_HEARTBEAT_URL}$1" || log "heartbeat ping failed"
}

trap 'log "base backup FAILED"; ping_heartbeat /fail' ERR

log "base backup started (data dir $PGDATA)"
wal-g backup-push "$PGDATA"
log "base backup finished"

log "retention: keep $BACKUP_RETENTION_FULL full backups"
wal-g delete retain FULL "$BACKUP_RETENTION_FULL" --confirm

wal-g backup-list --detail | tail -n 3
ping_heartbeat ""
