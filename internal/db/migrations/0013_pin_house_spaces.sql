-- Migration 13 ("pin house-visible spaces to dashboard"): extends Space
-- pinning (migration 12) to a House member who neither owns a Space nor
-- holds an explicit space_shares grant on it, but can still see it because
-- at least one of its tagged lists belongs to a House they're a member of
-- (see internal/db.spaceAccessibleViaHouse and the "Pinning shared lists
-- (and shared Spaces)" bullet in CLAUDE.md). space_shares itself can't carry
-- this preference: that table represents an explicit grant the Space's
-- owner made to one named recipient, and a fellow House member was never
-- granted anything by anyone — their visibility comes from House
-- membership, which is symmetric among every member and isn't something any
-- one person "shares". space_house_pins therefore has no permission column:
-- it stores nothing but the pin preference itself. Row presence means
-- pinned; there is no separate boolean flag, so unpinning deletes the row
-- rather than flipping it to 0 (unlike list_shares/space_shares'
-- is_pinned_to_dashboard, which flips a flag on a row that exists for other
-- reasons anyway).
CREATE TABLE space_house_pins (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    custom_category_id  INTEGER NOT NULL REFERENCES custom_categories(id) ON DELETE CASCADE,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (custom_category_id, user_id)
);

CREATE INDEX idx_space_house_pins_user_id ON space_house_pins(user_id);
