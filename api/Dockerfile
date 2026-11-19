# syntax=docker/dockerfile:1.7
# Mộc POS API + worker (one binary: mocpos serve | worker | migrate | seed).
# Build context: api/.   Multi-arch: docker buildx build --platform linux/arm64,linux/amd64 api
# The Go compiler runs natively on the build machine and cross-compiles (no QEMU needed for this stage).

ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
# timetzdata embeds the tz database so Asia/Ho_Chi_Minh works without OS tzdata.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags timetzdata \
      -ldflags "-s -w" -o /out/mocpos ./cmd/mocpos

# Small runtime with busybox wget for the compose health check. Runs as an unprivileged user.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 mocpos
COPY --from=build /out/mocpos /usr/local/bin/mocpos
USER 10001
ENV HTTP_ADDR=:8080 TZ=Asia/Ho_Chi_Minh
EXPOSE 8080
# Health check is defined per service in docker-compose.yml (only "serve" listens on HTTP).
ENTRYPOINT ["mocpos"]
CMD ["serve"]
