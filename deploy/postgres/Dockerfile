# syntax=docker/dockerfile:1.7
# PostgreSQL 16 + wal-g (continuous WAL archiving and nightly base backups to Cloudflare R2).
# Build context: repository root (needs deploy/backup/*.sh).
#   docker buildx build --platform linux/arm64,linux/amd64 -f deploy/postgres/Dockerfile .
# The same image runs the "postgres" service and the "backup" sidecar in docker-compose.yml.

FROM postgres:16-bookworm

ARG TARGETARCH
ARG WALG_VERSION=v3.0.7
# sha256 of the official release tarballs (github.com/wal-g/wal-g/releases), checked at build time.
ARG WALG_SHA256_AMD64=76d51ed915165d45314bc947300b9d1776adb2d875d857f580a730fd6f66900e
ARG WALG_SHA256_ARM64=fcf25aac732f66e77e121e9d287a08d8bf867c604b81cb6fcfff2d6c692d38c9

RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl; \
    rm -rf /var/lib/apt/lists/*; \
    case "$TARGETARCH" in \
      amd64) arch=amd64;   sum="$WALG_SHA256_AMD64" ;; \
      arm64) arch=aarch64; sum="$WALG_SHA256_ARM64" ;; \
      *) echo "unsupported arch $TARGETARCH"; exit 1 ;; \
    esac; \
    curl -fsSL -o /tmp/walg.tgz "https://github.com/wal-g/wal-g/releases/download/${WALG_VERSION}/wal-g-pg-ubuntu-22.04-${arch}.tar.gz"; \
    echo "$sum  /tmp/walg.tgz" | sha256sum -c -; \
    tar -xzf /tmp/walg.tgz -C /tmp; \
    install -m 0755 "/tmp/wal-g-pg-ubuntu-22.04-${arch}" /usr/local/bin/wal-g; \
    rm -f /tmp/walg.tgz "/tmp/wal-g-pg-ubuntu-22.04-${arch}"; \
    wal-g --version

COPY deploy/backup/*.sh /usr/local/bin/
RUN chmod 0755 /usr/local/bin/*.sh
