#!/usr/bin/env bash
# Sourced by the other backup scripts. Maps the friendly .env names to wal-g settings.
#
#   R2_ACCOUNT_ID, R2_BUCKET, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY   Cloudflare R2 (S3 compatible)
#   BACKUP_PREFIX           folder inside the bucket (default "mocpos")
#   BACKUP_ENCRYPTION_KEY   64 hex chars (openssl rand -hex 32). Every file is encrypted with
#                           libsodium before upload. LOSE THIS KEY = LOSE THE BACKUPS. Keep a copy offline.
#   BACKUP_LOCAL_DIR        (testing only) write backups to a local folder instead of R2
#
# Connection to the live server: PGHOST/PGPORT/PGUSER/PGPASSWORD/PGDATABASE (set in docker-compose.yml).

: "${BACKUP_PREFIX:=mocpos}"

if [ -n "${BACKUP_LOCAL_DIR:-}" ]; then
  export WALG_FILE_PREFIX="$BACKUP_LOCAL_DIR/$BACKUP_PREFIX"
else
  : "${R2_ACCOUNT_ID:?R2_ACCOUNT_ID is not set}"
  : "${R2_BUCKET:?R2_BUCKET is not set}"
  : "${R2_ACCESS_KEY_ID:?R2_ACCESS_KEY_ID is not set}"
  : "${R2_SECRET_ACCESS_KEY:?R2_SECRET_ACCESS_KEY is not set}"
  export WALG_S3_PREFIX="s3://$R2_BUCKET/$BACKUP_PREFIX"
  export AWS_ENDPOINT="https://$R2_ACCOUNT_ID.r2.cloudflarestorage.com"
  export AWS_REGION=auto
  export AWS_S3_FORCE_PATH_STYLE=true
  export AWS_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID"
  export AWS_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY"
fi

: "${BACKUP_ENCRYPTION_KEY:?BACKUP_ENCRYPTION_KEY is not set (openssl rand -hex 32)}"
if ! [[ "$BACKUP_ENCRYPTION_KEY" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "BACKUP_ENCRYPTION_KEY must be exactly 64 hex characters (openssl rand -hex 32)" >&2
  exit 1
fi
export WALG_LIBSODIUM_KEY="$BACKUP_ENCRYPTION_KEY"
export WALG_LIBSODIUM_KEY_TRANSFORM=hex
export WALG_COMPRESSION_METHOD=zstd
# Small VM: keep upload concurrency modest.
export WALG_UPLOAD_CONCURRENCY="${WALG_UPLOAD_CONCURRENCY:-4}"
export WALG_DOWNLOAD_CONCURRENCY="${WALG_DOWNLOAD_CONCURRENCY:-4}"

log() { echo "$(date '+%Y-%m-%d %H:%M:%S %z') [backup] $*"; }
