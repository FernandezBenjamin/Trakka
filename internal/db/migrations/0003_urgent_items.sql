-- Migration 3 ("Add Urgent items"): a plain, independently-settable flag —
-- unlike is_recurring it isn't derived from anything else — marking an item
-- as needing attention right away, applicable to both list types.
ALTER TABLE items ADD COLUMN is_urgent INTEGER NOT NULL DEFAULT 0;
