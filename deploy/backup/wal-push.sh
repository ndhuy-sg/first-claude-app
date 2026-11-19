#!/usr/bin/env bash
# PostgreSQL archive_command: wal-push.sh %p
# Ships one finished WAL segment to R2 (encrypted). With archive_timeout=300 at most ~5 minutes
# of changes are only on the VM disk (docs/ARCHITECTURE.md §8).
#
# When BACKUP_ENABLED is not "true" the segment is acknowledged and dropped, so PostgreSQL never
# piles up WAL on disk while backups are not configured yet (first install, CI, local runs).
set -euo pipefail
[ "${BACKUP_ENABLED:-false}" = "true" ] || exit 0
# shellcheck source=walg-env.sh
. /usr/local/bin/walg-env.sh
exec wal-g wal-push "$1"
