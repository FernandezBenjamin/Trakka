-- Trakka initial SQLite schema.
-- Applied idempotently at startup (CREATE ... IF NOT EXISTS), so it also
-- doubles as the migration for a fresh /data/trakka.db volume.

CREATE TABLE IF NOT EXISTS houses (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- lists.house_id is not part of this CREATE TABLE — it was added after the
-- table already had a shipped shape, so (like items.price) it's applied via
-- the addColumnIfMissing() guard in internal/db/db.go, right after this
-- schema is applied. See "Evolving the schema" in docs/DATABASE.md.
--
-- The `type` CHECK constraint was widened from ('shopping', 'todo') to add
-- 'groceries', 'recurring_shopping' and 'custom'. SQLite has no
-- `ALTER TABLE ... ADD/DROP CONSTRAINT`, so — unlike a new column — this
-- edit alone has no effect on a database file that already has this table;
-- migrateListsTypeCheck() in internal/db/db.go, run on every startup right
-- after this schema is applied, is what actually widens it on an existing
-- /data/trakka.db (by rebuilding the table — SQLite's only way to change a
-- CHECK constraint), same "never edit an existing CREATE TABLE in place and
-- expect it to reach existing deployments" caveat documented in
-- docs/DATABASE.md#evolving-the-schema, just for a constraint rather than a
-- column.
CREATE TABLE IF NOT EXISTS lists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'shopping' CHECK (type IN ('todo', 'shopping', 'groceries', 'recurring_shopping', 'custom')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    list_id    INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    url        TEXT,
    quantity   INTEGER NOT NULL DEFAULT 1,
    done       INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0, 1)),
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- items.due_date/is_recurring/recurrence_rule/recurrence_end_date/is_urgent
-- are not part of this CREATE TABLE either — same reasoning as
-- price/target_month above, applied via the addColumnIfMissing() guard in
-- internal/db/db.go.
CREATE INDEX IF NOT EXISTS idx_items_list_id ON items(list_id);
CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(type);

-- Local + OIDC user identities. password_hash is NULL for OIDC-only users;
-- oidc_subject/oidc_issuer are NULL for local-only users. A user must have
-- at least one way to authenticate.
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT,
    oidc_subject  TEXT,
    oidc_issuer   TEXT,
    display_name  TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (password_hash IS NOT NULL OR oidc_subject IS NOT NULL)
);

-- oidc_subject is only unique *within* an issuer, hence the composite
-- partial unique index rather than a plain UNIQUE column constraint.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_oidc_identity
    ON users(oidc_issuer, oidc_subject) WHERE oidc_subject IS NOT NULL;

-- Opaque server-side sessions. `id` stores SHA-256(raw cookie token) in
-- hex, never the raw token itself, so a database leak alone can't hand out
-- live sessions. expires_at uses the same strftime format as other
-- timestamp columns so it stays lexicographically comparable in SQL.
CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

-- House membership / access control. A house with zero rows here is
-- inaccessible to everyone via the API until someone is added.
CREATE TABLE IF NOT EXISTS house_members (
    house_id   INTEGER NOT NULL REFERENCES houses(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (house_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_house_members_user_id ON house_members(user_id);

-- Price-drop / better-deal alerts surfaced by internal/scraper's periodic
-- and on-demand price checks (see internal/handlers/price_alerts.go). A
-- pending alert means a lower price than the item's current price was found
-- at source_url; accepting one applies found_price to the item, rejecting
-- one just dismisses it without touching the item. original_price is a
-- snapshot of the item's price at the moment the alert was created, not
-- re-read from the item at accept/reject time, so a notification always
-- reflects what the comparison was actually made against even if the
-- item's price changes in the meantime. This is a brand new table (not an
-- evolution of an existing shipped one), so it needs no addColumnIfMissing
-- guard — CREATE TABLE IF NOT EXISTS alone is enough for it to appear on an
-- existing /data/trakka.db the same way it does on a fresh one.
CREATE TABLE IF NOT EXISTS price_alerts (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id        INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    original_price REAL NOT NULL,
    found_price    REAL NOT NULL,
    source_url     TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_price_alerts_item_id ON price_alerts(item_id);
CREATE INDEX IF NOT EXISTS idx_price_alerts_status ON price_alerts(status);

-- Per-user custom categories ("Spaces") for organizing lists beyond the
-- fixed lists.type enum — e.g. "Vacances", "Anniversaire de Léo". Owned by a
-- single user (user_id), unlike houses/lists which are shared: two members
-- of the same house each keep their own personal set of categories, and a
-- category attached to a shared list stays only editable/deletable by
-- whoever created it (see internal/handlers/categories.go's ownership
-- check). A brand new table, so (like price_alerts/system_settings above)
-- it needs no addColumnIfMissing guard — CREATE TABLE IF NOT EXISTS alone
-- is enough for it to appear on an existing /data/trakka.db the same way it
-- does on a fresh one.
CREATE TABLE IF NOT EXISTS custom_categories (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    icon       TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT '',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_custom_categories_user_id ON custom_categories(user_id);

-- lists.custom_category_id is not part of the `lists` CREATE TABLE above —
-- same reasoning as lists.house_id: it's applied via the
-- addColumnIfMissing() guard in internal/db/db.go, run after both this
-- table and migrateListsTypeCheck() exist/have run (see the comment at that
-- call site for why the ordering matters). ON DELETE SET NULL (rather than
-- CASCADE, like lists.house_id) is deliberate: deleting a custom category
-- should just unassign it from any list that used it, not delete the list
-- itself.

-- Dynamic runtime settings (OIDC configuration, open/closed registration,
-- instance name) that take priority over their equivalent environment
-- variable whenever a row exists here — see internal/settings.Resolve.
-- Managed exclusively through the admin-only PATCH /api/v1/admin/settings
-- endpoint; nothing else writes to this table. A brand new table, so (like
-- price_alerts above) it needs no addColumnIfMissing guard — CREATE TABLE
-- IF NOT EXISTS alone is enough for it to appear on an existing
-- /data/trakka.db the same way it does on a fresh one.
CREATE TABLE IF NOT EXISTS system_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
