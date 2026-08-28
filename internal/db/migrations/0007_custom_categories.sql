-- Migration 7 ("Add customs categories or groups"): custom_categories
-- ("Spaces") are a per-user, freeform way to organize lists beyond the
-- fixed lists.type enum. Owned by a single user (user_id) and never
-- shared, unlike houses — see internal/handlers/categories.go's ownership
-- check. lists.custom_category_id is how a list attaches to one;
-- ON DELETE SET NULL (rather than CASCADE) is deliberate, since deleting a
-- category should only unassign it from any list that used it, never
-- delete the list itself.
CREATE TABLE custom_categories (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    icon       TEXT NOT NULL DEFAULT '',
    color      TEXT NOT NULL DEFAULT '',
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_custom_categories_user_id ON custom_categories(user_id);

ALTER TABLE lists ADD COLUMN custom_category_id INTEGER REFERENCES custom_categories(id) ON DELETE SET NULL;

CREATE INDEX idx_lists_custom_category_id ON lists(custom_category_id);
