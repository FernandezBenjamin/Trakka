-- Migration 1: the schema as of Trakka's first tracked commit
-- ("Version alpha of trakka") — houses, lists, items, users, sessions, and
-- house_members, plus the columns/indexes that commit's code applied via
-- ALTER TABLE right after creating these tables (items.price/price_auto/
-- image_url/target_month, lists.house_id). Folding those into the initial
-- CREATE TABLE here is safe: this migration runs exactly once, against
-- either a genuinely empty database (building the real historical shape in
-- one step) or nothing at all (an existing database is adopted directly at
-- the latest version — see hasExistingSchema in internal/db/migrate.go —
-- so this file never actually runs against a database that already has
-- some of these tables but not others).
CREATE TABLE houses (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- type's CHECK constraint here is the original, narrower set; migration 6
-- (list_types_widen) widens it later, exactly as it did historically.
CREATE TABLE lists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'shopping' CHECK (type IN ('shopping', 'todo')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    house_id   INTEGER REFERENCES houses(id) ON DELETE CASCADE
);

CREATE TABLE items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    list_id    INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    url        TEXT,
    quantity   INTEGER NOT NULL DEFAULT 1,
    price      REAL,
    price_auto INTEGER NOT NULL DEFAULT 0,
    image_url  TEXT,
    target_month TEXT,
    done       INTEGER NOT NULL DEFAULT 0 CHECK (done IN (0, 1)),
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_items_list_id ON items(list_id);
CREATE INDEX idx_items_target_month ON items(target_month);
CREATE INDEX idx_lists_type ON lists(type);
CREATE INDEX idx_lists_house_id ON lists(house_id);

-- Local + OIDC user identities. password_hash is NULL for OIDC-only users;
-- oidc_subject/oidc_issuer are NULL for local-only users. A user must have
-- at least one way to authenticate.
CREATE TABLE users (
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
CREATE UNIQUE INDEX idx_users_oidc_identity
    ON users(oidc_issuer, oidc_subject) WHERE oidc_subject IS NOT NULL;

-- Opaque server-side sessions. `id` stores SHA-256(raw cookie token) in
-- hex, never the raw token itself, so a database leak alone can't hand out
-- live sessions. expires_at uses the same strftime format as other
-- timestamp columns so it stays lexicographically comparable in SQL.
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);

-- House membership / access control. A house with zero rows here is
-- inaccessible to everyone via the API until someone is added.
CREATE TABLE house_members (
    house_id   INTEGER NOT NULL REFERENCES houses(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    PRIMARY KEY (house_id, user_id)
);

CREATE INDEX idx_house_members_user_id ON house_members(user_id);
