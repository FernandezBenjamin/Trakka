# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: build a fully static binary, no CGO -> works with modernc.org/sqlite
# ---------------------------------------------------------------------------
# NOTE on the Go version: keep this in lockstep with go.mod's `go` directive
# and the modernc.org/sqlite version (see CLAUDE.md, "Go version pinning is
# intentional and fragile"). go.mod currently declares `go 1.27.0`, so the
# builder image cannot be downgraded below that — Go's own toolchain
# resolution would refuse to build. 1.27.0-alpine is already the most recent
# compatible patch release; bump this (and go.mod, and the sqlite driver
# version) together if a newer one ships.
FROM golang:1.27.0-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/trakka ./cmd/server

# Distroless has no shell/adduser, so the non-root passwd/group entries and
# the /data mount point are prepared here and copied into the runtime stage.
# Fixed numeric UID:GID 10001:10001 (not distroless's default 65532) is kept
# so this still maps cleanly under Podman rootless user-namespace remapping —
# see CLAUDE.md, "don't switch to a named USER without keeping the numeric
# UID pinned".
RUN addgroup -g 10001 -S trakka \
    && adduser -u 10001 -S trakka -G trakka -h /home/trakka -s /sbin/nologin \
    && mkdir -p /out/data && chown 10001:10001 /out/data

# ---------------------------------------------------------------------------
# Stage 2: distroless runtime — no shell, no package manager, static binary
# only. CA certificates (needed by internal/scraper's outbound HTTPS fetches)
# ship in this base image already; tzdata does not, which is fine since this
# app stores and logs every timestamp in UTC (see CLAUDE.md conventions) and
# never calls time.LoadLocation.
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:latest AS runtime
# Pin this to a specific digest (`docker pull` it once and read
# `RepoDigests`) before relying on this image for production reproducibility
# — "latest" here tracks Debian 12 rebuilds, not a stable release like the
# builder's Go version tag above.

COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /etc/group /etc/group
COPY --from=build --chown=10001:10001 /out/data /data

WORKDIR /app

COPY --from=build --chown=10001:10001 /out/trakka ./trakka
COPY --chown=10001:10001 static ./static
COPY --chown=10001:10001 templates ./templates

USER 10001:10001

ENV DB_PATH=/data/trakka.db \
    STATIC_DIR=/app/static \
    TEMPLATES_DIR=/app/templates \
    PORT=8080

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/trakka", "-healthcheck"]

ENTRYPOINT ["/app/trakka"]
