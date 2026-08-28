-- Migration 9 ("Add possibility to share lists or spaces"): granular
-- sharing layered on top of house membership — a Space (custom_categories
-- row) or an individual List can be shared directly with one other user,
-- giving them read/write access without adding them to the whole parent
-- House. See internal/db.AccessLevelForList and the "granular sharing"
-- bullet in CLAUDE.md for the full access-check design. Sharing a Space is
-- the owning user's call alone; sharing a List requires actual membership
-- of its parent House, so access granted via a share can never itself be
-- used to grant further access.
CREATE TABLE space_shares (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    custom_category_id  INTEGER NOT NULL REFERENCES custom_categories(id) ON DELETE CASCADE,
    shared_with_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission          TEXT NOT NULL DEFAULT 'read' CHECK (permission IN ('read', 'write')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (custom_category_id, shared_with_user_id)
);

CREATE INDEX idx_space_shares_user_id ON space_shares(shared_with_user_id);

CREATE TABLE list_shares (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    list_id             INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    shared_with_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission          TEXT NOT NULL DEFAULT 'read' CHECK (permission IN ('read', 'write')),
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (list_id, shared_with_user_id)
);

CREATE INDEX idx_list_shares_user_id ON list_shares(shared_with_user_id);
