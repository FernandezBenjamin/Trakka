# API Reference

Base URL: `http://<host>:<port>` (default port `8080`). All endpoints are under `/api/v1`.

All responses are `application/json; charset=utf-8`. Every `/api/v1/...` endpoint requires an authenticated session (see [Authentication](#authentication) below) — there is no anonymous access to the JSON API. `/auth/...` endpoints and static assets stay unauthenticated, since `/auth/...` is how a session gets established in the first place.

## Conventions

- **Errors** are always `{"error": "<message>"}` with a non-2xx status code.
- **Timestamps** (`created_at`, `updated_at`) are UTC, ISO-8601 with milliseconds, e.g. `2026-08-26T07:46:03.959Z`.
- **Request bodies** are JSON. Unknown fields in a request body are rejected (400). Body size is capped at 1 MiB.
- IDs are positive integers (SQLite `AUTOINCREMENT`).
- **URLs** (the item `url` field) must be an absolute `http://` or `https://` URL with a non-empty host, or omitted/empty. Anything else — `javascript:`, `data:`, a bare host with no scheme, a malformed string — is rejected with `400 {"error": "url must be an absolute http:// or https:// URL"}`.
- **Security headers**: every response (API and static) carries `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, a strict `Content-Security-Policy`, and `Referrer-Policy: no-referrer`.
- **Offline responses**: these are never sent by the Go backend itself — the service worker (`static/sw.js`) synthesizes them client-side when the network is unreachable. A `GET` served from the offline cache carries `X-Trakka-Offline: true`; a mutation accepted into the offline sync queue returns `202` with `X-Trakka-Queued: true` and an optimistic body (list/item ids look like `temp-list-<uuid>`/`temp-item-<uuid>` until they sync). See [docs/PWA.md](PWA.md).

## Authentication

Trakka supports two ways to establish a session, both ending in the same HTTP-only, `SameSite=Strict` `trakka_session` cookie:

- **Local**: email + password (bcrypt-hashed server-side).
- **OIDC / OAuth2**: a generic Authorization Code + PKCE flow against any standards-compliant provider (Authelia, Authentik, Keycloak, Google, ...), configured via `OIDC_ISSUER`/`OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET`/`BASE_URL` (see [docs/DEPLOYMENT.md](DEPLOYMENT.md)). Only enabled when all three `OIDC_*` variables are set.

These are classic server-rendered, form-POST endpoints (not JSON) — `/auth/login` doubles as the login *page* (`GET`) and the login *submit* target (`POST`), and successes/failures are full-page redirects, not JSON responses:

| Endpoint | Method | Notes |
|---|---|---|
| `/auth/login` | `GET` | Renders the login/register page. Redirects to `/` if already authenticated. |
| `/auth/login` | `POST` | Form fields: `email`, `password`. On success, sets the session cookie and redirects to `/`; on failure, redirects to `/auth/login?error=invalid_credentials`. |
| `/auth/register` | `POST` | Form fields: `email`, `password`, `password_confirm`, `display_name`. Creates the account, a personal house ("Ma Maison") owned by the new user, a session, then redirects to `/`. `email_taken` on a duplicate email. |
| `/auth/logout` | `POST` | Revokes the session and redirects to `/auth/login`. |
| `/auth/oidc/login` | `GET` | Redirects to the configured OIDC provider. `404` if OIDC isn't configured. |
| `/auth/oidc/callback` | `GET` | The provider's redirect target. Verifies the id_token, provisions the account on first login (with a personal house, same as local registration), sets the session cookie, redirects to `/`. |

A first-time OIDC login is rejected (`?error=email_taken`) if the claimed email already belongs to a different account (local, or a different OIDC issuer) — accounts are never silently auto-linked by email, since an OIDC provider's email claim isn't guaranteed to be verified.

```bash
# example: local login via curl, using a cookie jar
curl -c cookies.txt -X POST http://localhost:8080/auth/login -d "email=alice@example.com&password=secret1234"
curl -b cookies.txt http://localhost:8080/api/v1/me
```

### `GET /api/v1/me`

Returns the authenticated user. `401 {"error": "authentication required"}` if the session cookie is missing, invalid, or expired.

```json
{ "id": 1, "email": "alice@example.com", "display_name": "Alice", "created_at": "..." }
```

**CSRF**: the login/register forms carry no CSRF token — there's no pre-existing session for a forged POST to hijack at that point, so there's nothing for an attacker to gain. Every subsequent state-changing call goes through `/api/v1/...`, which is protected by the session cookie's `SameSite=Strict` attribute: it's never attached to a cross-site request (form or `fetch`), which closes the standard CSRF vector without a separate token.

## Health

### `GET /healthz`

Pings the database. Used by the container `HEALTHCHECK`.

- `200` `{"status": "ok"}`
- `503` `{"error": "database unavailable"}`

## Houses

A House groups related lists together, shared by the members of a household. Every list belongs to exactly one house, and every house has one or more members via `house_members` (`owner` or `member` role) — a house with zero members is unreachable via the API to everyone (this can't normally happen: houses are only ever created together with their owner, see below).

**Access control**: a user can only see and modify houses (and their lists/items) they are a member of. `GET`/access-checked endpoints return `403 {"error": "not a member of this house"}` for a house the caller isn't in; owner-only actions (rename, delete, invite, remove another member) return `403 {"error": "only the house owner can do this"}` for a non-owner member. Every house object in a response carries a `role` field — the caller's own role in that house — so the frontend can conditionally show owner-only controls.

Every new user (on registration or first OIDC login) gets a personal house ("Ma Maison") created automatically with them as owner, so there's always at least one house to land on.

### `GET /api/v1/houses`

Returns the houses the caller is a member of, oldest first.

```bash
curl -b cookies.txt http://localhost:8080/api/v1/houses
```

```json
[
  { "id": 2, "name": "Ma Maison", "created_at": "...", "role": "owner" }
]
```

### `POST /api/v1/houses`

Creates a house with the caller as its owner (atomically — a house is never left without a member).

```bash
curl -b cookies.txt -X POST http://localhost:8080/api/v1/houses -d '{"name": "Maison des Parents"}'
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | trimmed; `400` if empty |

`201` with the created house (`role: "owner"`).

### `GET /api/v1/houses/{id}`

`200` with the house (including the caller's `role`), `403` if the caller isn't a member, `404` if not found. Lists belonging to it are fetched separately via `GET /api/v1/lists?house_id={id}`, not embedded here.

### `PUT /api/v1/houses/{id}`

Renames a house. Owner-only. `403` if the caller isn't the owner, `404` if not found, `400` if `name` is empty.

### `DELETE /api/v1/houses/{id}`

Owner-only. `204` on success. Deleting a house **cascades** to delete all its lists (and therefore all their items) and all its `house_members` rows (`ON DELETE CASCADE`, transitively). `403` if the caller isn't the owner, `404` if not found.

## House members

Membership is how a house is shared between users. There's no email-sending infrastructure in this project, so inviting someone only works once they already have a Trakka account (local or OIDC) — invite-by-email fails clearly (`404`) rather than creating an unredeemable pending invite.

### `GET /api/v1/houses/{id}/members`

Any member can view the roster. Owners are listed first.

```bash
curl -b cookies.txt http://localhost:8080/api/v1/houses/2/members
```

```json
[
  { "house_id": 2, "user_id": 1, "role": "owner", "created_at": "...", "email": "alice@example.com", "display_name": "Alice" },
  { "house_id": 2, "user_id": 2, "role": "member", "created_at": "...", "email": "bob@example.com", "display_name": "Bob" }
]
```

`403` if the caller isn't a member of the house.

### `POST /api/v1/houses/{id}/members`

Owner-only. Invites an existing user by email.

```bash
curl -b cookies.txt -X POST http://localhost:8080/api/v1/houses/2/members -d '{"email": "bob@example.com"}'
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `email` | string | yes | must belong to an existing account, else `404` |

`201` with the created membership (`role: "member"`). `403` if the caller isn't the owner. `404` if no account exists for that email. `409` if already a member.

### `DELETE /api/v1/houses/{id}/members/{userId}`

Removes a member. A user may always remove **themselves** ("leave house") regardless of role; removing anyone else requires being the owner.

`204` on success. `403` if the caller lacks permission. `404` if the target user isn't a member.

## Lists

A list has a `type` of either `shopping` or `todo`, and belongs to exactly one house (`house_id`).

### `GET /api/v1/lists`

Returns lists belonging to houses the caller is a member of, newest first. Items are **not** embedded here (see `GET /api/v1/lists/{id}` for that).

Query parameters:
- `type` (optional) — filter to `shopping` or `todo`. `400` if any other value is given.
- `house_id` (optional) — filter to lists belonging to that house. `400` if not a positive integer; `403` if the caller isn't a member of that house.

```bash
curl -b cookies.txt http://localhost:8080/api/v1/lists
curl -b cookies.txt http://localhost:8080/api/v1/lists?type=shopping
curl -b cookies.txt http://localhost:8080/api/v1/lists?house_id=1
```

```json
[
  { "id": 1, "house_id": 1, "name": "Courses", "type": "shopping", "created_at": "...", "updated_at": "..." }
]
```

### `POST /api/v1/lists`

```bash
curl -X POST http://localhost:8080/api/v1/lists \
  -d '{"name": "Courses", "type": "shopping", "house_id": 1}'
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | trimmed; `400` if empty |
| `type` | string | no | `shopping` (default) or `todo`; `400` if anything else |
| `house_id` | integer | yes | must reference an existing house, else `400` |

`201` with the created list (no `items` field).

### `GET /api/v1/lists/{id}`

Returns the list **with its items embedded**, ordered by `position` then `id`.

- `200` with the list, e.g.:
  ```json
  {
    "id": 1, "house_id": 1, "name": "Courses", "type": "shopping",
    "created_at": "...", "updated_at": "...",
    "items": [
      { "id": 1, "list_id": 1, "title": "Lait", "url": "https://...", "quantity": 2, "price": 1.85, "price_auto": false, "image_url": "https://...", "done": false, "position": 0, "target_month": "2026-11", "created_at": "...", "updated_at": "..." }
    ]
  }
  ```
- `404` if the list doesn't exist. `403` if the caller isn't a member of the list's house.

### `PUT /api/v1/lists/{id}`

Full replace of `name` and `type` (same validation as `POST`). `house_id` cannot be changed via this endpoint. Returns the updated list (no `items`). `404` if not found, `403` if the caller isn't a member of the list's house.

### `DELETE /api/v1/lists/{id}`

`204` on success. Deleting a list **cascades** to delete all its items (`ON DELETE CASCADE`). `404` if not found, `403` if the caller isn't a member of the list's house.

## Items

### `GET /api/v1/items?list_id={id}`

`list_id` is required. Returns `200` with an array. `400` if `list_id` is missing, not a positive integer, or doesn't reference an existing list. `403` if the caller isn't a member of the list's house.

```bash
curl -b cookies.txt "http://localhost:8080/api/v1/items?list_id=1"
```

### `POST /api/v1/items`

```bash
curl -X POST http://localhost:8080/api/v1/items \
  -d '{"list_id": 1, "title": "Lait", "url": "https://example.com/lait", "quantity": 2, "price": 1.85}'
```

| Field | Type | Required | Notes |
|---|---|---|---|
| `list_id` | integer | yes | must reference an existing list, else `400` |
| `title` | string | yes | trimmed; `400` if empty |
| `url` | string | no | must be an absolute `http://`/`https://` URL if given, else `400`; empty string is stored as `null` |
| `quantity` | integer | no | defaults to `1` if omitted or `<= 0` |
| `price` | number | no | unit price in euros; `400` if negative; omitted/`null` means no price recorded |
| `position` | integer | no | defaults to `0`; used for manual ordering within a list |
| `target_month` | string | no | planned purchase month, `YYYY-MM` (e.g. `"2026-11"`); `400` if given but not in that format; empty string is stored as `null` (unscheduled) |

`201` with the created item. If `url` is given, the server kicks off a **best-effort** lookup against that URL (see `internal/scraper` in [CLAUDE.md](../CLAUDE.md)) for whichever of `price`/`image_url` the item is still missing (an explicit `price` in the request skips price detection but the image is still looked up), and waits up to ~2.5s for it before responding: if it finishes in time, the `201` response already carries `price` (with `price_auto: true`) and `image_url`, and reports `price_status: "found"`; if the site is slow to respond, the lookup keeps running in the background and the response instead carries `price_status: "pending"` — poll `GET /api/v1/items/{id}` (or re-fetch the list) a few seconds later to pick it up. `price_status: "none"` means there was no `url` to scrape or no price was found in time — this is unaffected by whether an image was found. `image_url`, when present, is always an absolute `http://`/`https://` URL. `price_status` is response-only — it's never persisted and never appears on a plain `GET`.

### `GET /api/v1/items/{id}`

`200` with the item, `404` if not found, `403` if the caller isn't a member of the item's list's house.

### `PUT /api/v1/items/{id}`

Full replace — every field below is required except `url`, `quantity`, `price`, `done`, `position`, `target_month` fall back to their zero/default value if omitted (note: unlike `PATCH`, omitting a field here **resets** it, since this is a full replace — an omitted `price` clears any previously recorded price, and an omitted `target_month` unschedules the item). `list_id` cannot be changed via this endpoint.

```bash
curl -X PUT http://localhost:8080/api/v1/items/1 \
  -d '{"title": "Lait", "url": "https://example.com/lait", "quantity": 2, "price": 1.85, "done": true, "position": 0, "target_month": "2026-11"}'
```

`404` if not found. `400` if `price` is negative or `target_month` isn't `YYYY-MM`. `403` if the caller isn't a member of the item's list's house. `price_auto` is always reset to `false` by this endpoint (a full replace is always an explicit, manual value, even when `price` is omitted). If `url` changes to something new, `image_url` is cleared (a scraped image is tied to the `url` it was found on) and the same bounded lookup as `POST` above kicks off for whichever of `price`/`image_url` is still missing, with the same `price_status` values in the response.

### `PATCH /api/v1/items/{id}`

Partial update — only send the fields you want to change. This is the endpoint to use for e.g. checking an item off a list, or moving a planned purchase to a different month:

```bash
curl -X PATCH http://localhost:8080/api/v1/items/1 -d '{"done": true}'
```

All fields (`title`, `url`, `quantity`, `price`, `done`, `position`, `target_month`) are optional. `title`, if given, cannot be empty; `quantity`, if given, must be positive; `price`, if given, must be a non-negative number or `null` (unlike the other fields, `price` distinguishes "omitted" — leave unchanged — from an explicit `null` — clear the recorded price). Sending `price` (either a number or `null`) always resets `price_auto` to `false`, since a `price` supplied in the request is by definition a manual value. `target_month`, if given, must be `YYYY-MM` or an empty string (clears it back to unscheduled) — omitting it entirely leaves the item's schedule untouched. There is no `image_url` field to set directly — it's scraper-only. If `url` changes to something new, `image_url` is cleared (same reasoning as `PUT` above) and the bounded lookup described under `POST` kicks off for whichever of `price`/`image_url` is still missing, with the same `price_status` values in the response; an unrelated `PATCH` (e.g. `{"done": true}`) never re-triggers it, since `url` isn't changing. `404` if not found, `403` if the caller isn't a member of the item's list's house.

```bash
# clear a previously recorded price without touching anything else
curl -X PATCH http://localhost:8080/api/v1/items/1 -d '{"price": null}'

# reschedule a planned purchase to a different month
curl -X PATCH http://localhost:8080/api/v1/items/1 -d '{"target_month": "2027-01"}'
```

### `DELETE /api/v1/items/{id}`

`204` on success, `404` if not found, `403` if the caller isn't a member of the item's list's house.

## Static assets

Everything under `static/` is served from `/` (e.g. `static/index.html` → `/`, `static/js/app.js` → `/js/app.js`). This is unrelated to the JSON API but shares the same HTTP server and the same security headers.
