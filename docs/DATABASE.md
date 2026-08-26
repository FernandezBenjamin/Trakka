# Database

Trakka uses SQLite via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite), a pure-Go driver — there is no CGO dependency on `libsqlite3`, which is what keeps the binary static and the image minimal.

## Location & connection

- Path is set by the `DB_PATH` environment variable (default `/data/trakka.db` in the container, `./trakka.db` for local runs).
- Opened with: `journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`.
- The connection pool is capped at **one connection** (`SetMaxOpenConns(1)` / `SetMaxIdleConns(1)`) in `Open()` in [internal/db/db.go](../internal/db/db.go). SQLite allows only one writer at a time regardless of pool size, and the pure-Go driver gains nothing from pooling multiple readers the way `mattn/go-sqlite3` might — a single shared connection avoids `SQLITE_BUSY` errors under concurrent requests. Do not raise this without a specific reason.
- All queries live in [internal/db/lists.go](../internal/db/lists.go), [internal/db/items.go](../internal/db/items.go), [internal/db/houses.go](../internal/db/houses.go), [internal/db/house_members.go](../internal/db/house_members.go), [internal/db/users.go](../internal/db/users.go) and [internal/db/sessions.go](../internal/db/sessions.go), exclusively as parameterized statements (`?` placeholders) — no other package builds SQL or imports `database/sql`.
- `CreateHouseWithOwner` (in `houses.go`) is the one place in this package that opens an explicit `sql.Tx` — it needs the house-row insert and the owner's `house_members` insert to succeed or fail together, so a house is never left without an owner. The single-connection pool above means this simply serializes against other writes; it doesn't change the pooling rules.

## Schema

Defined in [internal/db/schema.sql](../internal/db/schema.sql), embedded into the binary at build time via `//go:embed` and executed on every startup. Every statement is idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`), so this file **is** the migration mechanism — there's no separate migration tool or version table.

### `houses`

| Column | Type | Notes |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | |
| `name` | `TEXT NOT NULL` | |
| `created_at` | `TEXT NOT NULL` | ISO-8601 UTC, set by `strftime` default |

Groups related lists together. `ensureDefaultHouse()` (`internal/db/db.go`, run right after the schema and the `lists.house_id` migration below, on every startup) inserts a house named "Maison Principale" the first time no house exists at all, and backfills `house_id` on any list that doesn't have one yet — which is every list on an upgrade from before this table existed. This is what lets `house_id` be treated as effectively required at the application level (see below) despite being a nullable column at the SQL level.

Since the `house_members` table (below) was added, every house also needs an owner to be reachable via the API at all — `CreateHouseWithOwner()` (`internal/db/houses.go`) is now the only way houses get created through normal use (via `POST /api/v1/houses`, registration, or first OIDC login), and always adds an owner atomically. `ensureDefaultHouse()`'s "Maison Principale" seed predates that requirement and is **not** given an owner, so on a fresh database it becomes a permanently orphaned row: nobody is a member of it, so it's invisible to every RBAC-scoped query and can never be renamed or deleted via the API either. This is harmless (one dead row, no data exposure) and deliberately left as-is rather than special-cased, since it's a leftover from before per-user houses existed and nothing in the app depends on it.

### `lists`

| Column | Type | Notes |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | |
| `house_id` | `INTEGER` | `REFERENCES houses(id) ON DELETE CASCADE`; nullable at the SQL level only until `ensureDefaultHouse()` backfills it — see below |
| `name` | `TEXT NOT NULL` | |
| `type` | `TEXT NOT NULL DEFAULT 'shopping'` | `CHECK (type IN ('shopping', 'todo'))` |
| `created_at` | `TEXT NOT NULL` | ISO-8601 UTC, set by `strftime` default |
| `updated_at` | `TEXT NOT NULL` | updated explicitly by handlers on write |

### `items`

| Column | Type | Notes |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | |
| `list_id` | `INTEGER NOT NULL` | `REFERENCES lists(id) ON DELETE CASCADE` |
| `title` | `TEXT NOT NULL` | |
| `url` | `TEXT` | nullable; used for shopping-list product links |
| `quantity` | `INTEGER NOT NULL DEFAULT 1` | |
| `price` | `REAL` | nullable; unit price in euros, optional, primarily used on `shopping` lists |
| `price_auto` | `INTEGER NOT NULL DEFAULT 0` | booleans stored as 0/1, same convention as `done`; `1` when `price` was filled in by `internal/scraper`'s background lookup rather than typed in by a user — see [API.md](API.md#post-apiv1items) |
| `image_url` | `TEXT` | nullable; product image found by the same background lookup as `price` (og:image/JSON-LD/twitter:image). Scraper-only — there is no request field to set it manually — and cleared back to `NULL` whenever `url` changes to something new, since an image found for the previous `url` no longer describes the item |
| `target_month` | `TEXT` | nullable; planned purchase month in `YYYY-MM` form (validated by `internal/validate.Month`), used by the "Budget & Prévisions Achats" planning view (`static/js/planning.js`) to group and total upcoming spending — see [API.md](API.md#post-apiv1items) |
| `done` | `INTEGER NOT NULL DEFAULT 0` | `CHECK (done IN (0, 1))` — booleans are stored as 0/1, converted to Go `bool` in `scanItem()` |
| `position` | `INTEGER NOT NULL DEFAULT 0` | manual ordering within a list |
| `due_date` | `TEXT` | nullable; `YYYY-MM-DD` (validated by `internal/validate.Date`). For a recurring item, this is the current occurrence's due date, advanced automatically on completion rather than edited directly — see `recurrence_rule` below and [API.md](API.md#recurring-items) |
| `is_recurring` | `INTEGER NOT NULL DEFAULT 0` | booleans stored as 0/1, same convention as `done`/`price_auto`. Not settable independently — always written as `recurrence_rule IS NOT NULL` (see `CreateItem`/`UpdateItem` in `internal/db/items.go`), so it can never disagree with `recurrence_rule` |
| `recurrence_rule` | `TEXT` | nullable; one of `DAILY`/`WEEKLY`/`MONTHLY`/`YEARLY` or the custom `EVERY_X_DAYS:<n>` form (validated by `internal/validate.Recurrence`). `NULL` means the item doesn't repeat |
| `recurrence_end_date` | `TEXT` | nullable; `YYYY-MM-DD` (validated by `internal/validate.Date`) — the last date a recurring item should recur on. Once the next computed occurrence would fall after it, the item stops advancing and simply stays `done` |
| `created_at` | `TEXT NOT NULL` | |
| `updated_at` | `TEXT NOT NULL` | |

### `users`

| Column | Type | Notes |
|---|---|---|
| `id` | `INTEGER PRIMARY KEY AUTOINCREMENT` | |
| `email` | `TEXT NOT NULL UNIQUE COLLATE NOCASE` | case-insensitive uniqueness |
| `password_hash` | `TEXT` | nullable; bcrypt hash, `NULL` for OIDC-only accounts |
| `oidc_subject` | `TEXT` | nullable; the provider's `sub` claim |
| `oidc_issuer` | `TEXT` | nullable; the provider's issuer URL |
| `display_name` | `TEXT NOT NULL DEFAULT ''` | |
| `created_at` | `TEXT NOT NULL` | ISO-8601 UTC, set by `strftime` default |

`CHECK (password_hash IS NOT NULL OR oidc_subject IS NOT NULL)` — every user must have at least one way to authenticate. A unique partial index, `idx_users_oidc_identity` on `(oidc_issuer, oidc_subject) WHERE oidc_subject IS NOT NULL`, enforces that an OIDC identity is unique *within* its issuer (not globally, since two different providers could coincidentally reuse the same `sub` string).

### `sessions`

| Column | Type | Notes |
|---|---|---|
| `id` | `TEXT PRIMARY KEY` | hex-encoded SHA-256 of the raw cookie token — **never the raw token itself**, so a database leak alone can't hand out live sessions |
| `user_id` | `INTEGER NOT NULL` | `REFERENCES users(id) ON DELETE CASCADE` |
| `expires_at` | `TEXT NOT NULL` | same `strftime` format as other timestamps, so it stays lexicographically comparable to `strftime('now')` in the lookup query |
| `created_at` | `TEXT NOT NULL` | |

Indexed by `user_id` (`idx_sessions_user_id`). Deleting a user cascades to delete all their sessions (logged out everywhere).

### `house_members`

| Column | Type | Notes |
|---|---|---|
| `house_id` | `INTEGER NOT NULL` | `REFERENCES houses(id) ON DELETE CASCADE` |
| `user_id` | `INTEGER NOT NULL` | `REFERENCES users(id) ON DELETE CASCADE` |
| `role` | `TEXT NOT NULL DEFAULT 'member'` | `CHECK (role IN ('owner', 'member'))` |
| `created_at` | `TEXT NOT NULL` | |

`PRIMARY KEY (house_id, user_id)` — a user can only have one membership row per house. Indexed additionally by `user_id` (`idx_house_members_user_id`), for the "which houses is this user in" queries the RBAC layer runs on every request. This is the join table access control (`internal/handlers` — see `authorizeHouseAccess`/`authorizeHouseOwner`) is built on: a house's lists/items are only visible to its members, and only its owner can rename/delete the house or manage its membership. See [docs/API.md](API.md#houses) for the resulting endpoint behavior.

Indexes: `idx_items_list_id` (on `items.list_id`), `idx_items_target_month` (on `items.target_month`), `idx_lists_type` (on `lists.type`), `idx_lists_house_id` (on `lists.house_id`), `idx_sessions_user_id`, `idx_house_members_user_id`, `idx_users_oidc_identity` (unique, partial).

`price`/`price_auto`/`image_url`/`target_month`/`due_date`/`is_recurring`/`recurrence_rule`/`recurrence_end_date` (on `items`) and `house_id` (on `lists`) are not part of their tables' `CREATE TABLE` statements in `schema.sql` — each was added after its table already had a shipped shape, so each is applied via the `ALTER TABLE ... ADD COLUMN` guard in `addColumnIfMissing()` (`internal/db/db.go`), run at startup right after the embedded schema. That function is the concrete implementation of the "guard it in Go via `PRAGMA table_info`" approach described below; any future new column on an existing table should follow the same pattern rather than editing the `CREATE TABLE` in place. `idx_lists_house_id` and `idx_items_target_month` are likewise created in Go (not in `schema.sql`) since each must run after its `ALTER TABLE` — on a fresh database, `schema.sql` alone creates `lists`/`items` without `house_id`/`target_month` yet. `users`, `sessions`, and `house_members` are brand-new tables added directly in `schema.sql`, so none of them needed this Go-side guard.

Deleting a row from `houses` cascades to delete all its `lists` (which in turn cascades to delete all their `items`) and all its `house_members` rows — requiring `foreign_keys=ON` (already set on every connection), and working transitively without any extra code. Deleting a `user` cascades to their `sessions` and `house_members` rows; houses they owned are **not** deleted (a house survives its owner's account being removed, though it may then be left without an owner — that's an acknowledged edge case, not actively guarded against, since account deletion isn't exposed via the API today).

## Evolving the schema

Because `schema.sql` runs on every startup against a potentially already-populated `/data/trakka.db`, changes must be additive and idempotent:

- New tables/indexes: `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`.
- New columns on an existing table: cannot use `IF NOT EXISTS` in SQLite for `ALTER TABLE ... ADD COLUMN`; guard it in Go (e.g. query `PRAGMA table_info(items)` and conditionally `ALTER TABLE`) or accept that it only applies to fresh databases and document the manual migration step.
- Never edit an existing `CREATE TABLE` statement in place — it has no effect on a database file that already has that table, so existing deployments would silently diverge from fresh ones.

## Backup

The whole database is the single file at `DB_PATH` (plus its `-wal`/`-shm` sidecar files while the process is running, due to WAL mode). To back up safely while Trakka is running, either:
- use SQLite's own backup mechanism (e.g. `sqlite3 /data/trakka.db ".backup /path/to/copy.db"`), or
- stop the container and copy the volume's contents.

Copying `trakka.db` alone while the process is live, without also capturing `trakka.db-wal`, can produce an inconsistent copy.
