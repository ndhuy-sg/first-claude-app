#!/usr/bin/env bash
# Entry point of the "backup" sidecar: one base backup every night at BACKUP_TIME
# (Vietnam time, default 02:30 — far from the 06:30–09:00 rush), plus one right away when
# the bucket holds no backup yet. WAL is shipped continuously by the postgres service itself.
set -euo pipefail
export TZ="${TZ:-Asia/Ho_Chi_Minh}"
: "${BACKUP_TIME:=02:30}"

log() { echo "$(date '+%Y-%m-%d %H:%M:%S %z') [backup] $*"; }

if [ "${BACKUP_ENABLED:-false}" != "true" ]; then
  log "BACKUP_ENABLED is not true: backups are OFF. Set it in .env and run: docker compose up -d"
  exec sleep infinity
fi
. /usr/local/bin/walg-env.sh

until pg_isready -q; do log "waiting for postgres"; sleep 5; done

if ! wal-g backup-list >/dev/null 2>&1 || [ "$(wal-g backup-list 2>/dev/null | wc -l)" -le 1 ]; then
  log "no base backup in the bucket yet: taking the first one now"
  base-backup.sh || log "first base backup failed, will retry at $BACKUP_TIME"
fi

while true; do
  now=$(date +%s)
  next=$(date -d "today $BACKUP_TIME" +%s)
  [ "$next" -gt "$now" ] || next=$(date -d "tomorrow $BACKUP_TIME" +%s)
  log "next base backup at $(date -d "@$next" '+%Y-%m-%d %H:%M %z')"
  sleep $((next - now))
  base-backup.sh || log "base backup failed (see above); retrying tomorrow"
done
