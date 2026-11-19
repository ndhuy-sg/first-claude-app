#!/usr/bin/env bash
# PostgreSQL restore_command during recovery: wal-fetch.sh %f %p
set -euo pipefail
# shellcheck source=walg-env.sh
. /usr/local/bin/walg-env.sh
exec wal-g wal-fetch "$1" "$2"
