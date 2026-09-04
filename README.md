<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="static/icons/trakka-lockup-dark-bg.svg">
    <img alt="Trakka" src="static/icons/trakka-lockup-light-bg.svg" height="64">
  </picture>
</p>

Ultra-lightweight shopping-list and to-do API, written in Go. Single static binary, pure-Go SQLite (no CGO), targets **< 20 MB RAM** at runtime, and runs identically under Docker and Podman (rootless).

## Features

- REST JSON API for lists (`shopping` or `todo`) and their items (with optional URLs, quantities, done/checked state)
- Pure Go, no CGO — one static binary, no `libsqlite3` dependency
- Runs as a non-root, fixed-UID container under both Docker and Podman rootless
- In-process healthcheck (no `curl`/`wget` needed in the image)
- Installable, offline-first PWA on iOS, iPadOS, and Android: app-shell caching, an IndexedDB-backed sync queue for offline edits, vanilla ES6+ JS with no dependencies
- Optional CalDAV sync via a companion [Radicale](https://radicale.org/) service
- Touch-friendly dark-mode dashboard (Tailwind CSS via CDN): live network-status indicator, "Achats & Sourcing" / "Espaces Tâches" sections with dynamic cards (URL-sourcing badges, completion progress), a modal to create new lists
- Security-hardened by construction: parameterized SQL only, strict `http(s)://`-only URL validation, and standard hardening headers (`X-Content-Type-Options`, `X-Frame-Options`, a `Content-Security-Policy` that's `unsafe-inline`-free except for the one exception the Tailwind CDN script requires — see [CLAUDE.md](CLAUDE.md#security-rules)) on every response

## Quick start

### Docker / Podman

```bash
docker compose up -d --build
# or
podman-compose up -d
```

The API is then available at `http://localhost:8080`. Data persists in the `trakka_data` named volume.

To also start the optional CalDAV sync server:

```bash
docker compose --profile calendar up -d
```

### Local (no container)

Requires Go 1.27.0+.

```bash
go build -o trakka ./cmd/server
DB_PATH=./trakka.db STATIC_DIR=./static ./trakka
```

## Documentation

- [docs/API.md](docs/API.md) — full REST API reference
- [docs/DATABASE.md](docs/DATABASE.md) — SQLite schema and data model
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — Docker/Podman deployment, configuration, rootless notes
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) — local dev setup, building, project layout
- [docs/PWA.md](docs/PWA.md) — offline support: service worker, IndexedDB sync queue, iOS/Android specifics
- [docs/INSTALLATION.md](docs/INSTALLATION.md) — step-by-step guide to installing Trakka as an app on iOS/iPadOS and Android
- [docs/DOC_PUSH_NOTIFICATIONS.md](docs/DOC_PUSH_NOTIFICATIONS.md) — configuring VAPID keys, HTTPS/background-worker prerequisites, and end-to-end troubleshooting for Web Push notifications

## Tech stack

| Component | Choice | Why |
|---|---|---|
| HTTP server | `net/http` (stdlib) | No router dependency; Go 1.22+ method-based `ServeMux` patterns are enough |
| Database | [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | Pure Go SQLite driver — no CGO, so the binary stays static and portable |
| Logging | `log/slog` (stdlib) | Structured JSON logs with no dependency |
| CSS | [Tailwind Play CDN](https://tailwindcss.com/docs/installation/play-cdn) | Zero build step; the one deliberate CSP exception (see [CLAUDE.md](CLAUDE.md#security-rules)) — a precompiled Tailwind build is the lower-footprint alternative if that trade-off ever needs revisiting |
| Container base | `golang:1.27.0-alpine` (build) → `alpine:latest` (runtime) | Minimal image size, multi-stage build |

## Project layout

```
cmd/server/           entry point: config, wiring, graceful shutdown
internal/config/      environment-variable configuration
internal/models/      shared List/Item types
internal/db/          all SQLite access (parameterized queries only)
internal/handlers/    HTTP routing, validation, security headers
internal/validate/    reusable input validators (URL scheme, ...)
static/               PWA frontend: index.html, css/ (tokens.css + base.css), js/ (app.js + db.js), sw.js, manifest.json, icons/
```

## PWA / offline support

Trakka installs and works offline on iOS, iPadOS, and Android. This requires HTTPS in production (service workers don't register over plain HTTP except on `localhost`) — see [docs/PWA.md](docs/PWA.md) for the full picture, including why and how offline-created lists/items get reconciled once the network comes back.

## Visual identity

Trakka's mark is "Le T-Coche": a geometric T whose vertical stroke breaks into a diagonal checkmark, with an emerald dot at the tip standing for sync/online status.

| Role | Hex |
|---|---|
| Background | `#0f172a` |
| Deep background | `#0b0f19` |
| Surface / cards | `#1e293b` |
| Accent — indigo | `#6366f1` |
| Accent — violet | `#8b5cf6` |
| Symbol gradient | 135°, `#8b5cf6` → `#6366f1` |
| Success / online | `#10b981` (light `#34d399`) |
| Text | `#f8fafc` · muted `#94a3b8` · subtle `#64748b` |

These are the same values exposed as CSS custom properties in [static/css/tokens.css](static/css/tokens.css) (`--tk-bg`, `--tk-accent`, `--tk-success`, ...), so the app UI and the brand palette never drift apart.

**Emerald is a status signal, not a decorative color** — it's reserved for completed/validated states and "online" badges. Using it elsewhere drains it of meaning.

Logo usage:

- Minimum size: **16px** for the symbol alone, **96px wide** for the horizontal lockup — below that, use the symbol alone rather than an illegible lockup.
- Clear space around the lockup: the height of the T's stroke on all four sides.
- Use `trakka-lockup-light-bg.svg` on light backgrounds and `trakka-lockup-dark-bg.svg` on dark ones (see [static/icons/](static/icons/)) — don't recolor the symbol, add shadows/outlines/glow, or stretch it non-proportionally. The lockup word is already vectorized to curves, so no typeface is required to render it.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to set up a dev environment, the project's coding conventions, and the pull request process.

## License

See [LICENSE](LICENSE).
