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
CREATE TABLE IF NOT EXISTS lists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'shopping' CHECK (type IN ('shopping', 'todo')),
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
