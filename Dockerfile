# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: build a fully static binary, no CGO -> works with modernc.org/sqlite
# ---------------------------------------------------------------------------
FROM golang:1.27.0-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/trakka ./cmd/server

# ---------------------------------------------------------------------------
# Stage 2: minimal, non-root runtime image
# ---------------------------------------------------------------------------
FROM alpine:latest AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 10001 -S trakka \
    && adduser -u 10001 -S trakka -G trakka -h /home/trakka -s /sbin/nologin \
    && mkdir -p /data /app/static /app/templates \
    && chown -R trakka:trakka /data /app

WORKDIR /app

COPY --from=build --chown=trakka:trakka /out/trakka ./trakka
COPY --chown=trakka:trakka static ./static
COPY --chown=trakka:trakka templates ./templates

# Rootless-friendly: run as a fixed numeric UID/GID so it maps cleanly under
# both `docker run --user` and Podman's rootless user-namespace remapping.
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
