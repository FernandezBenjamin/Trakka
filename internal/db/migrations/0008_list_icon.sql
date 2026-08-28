-- Migration 8 ("Add categories and spaces organisation..."): a list's own
-- freeform display icon (typically an emoji, e.g. 🛒/🖥️/📦), same
-- convention as custom_categories.icon. Falls back to a fixed per-type
-- default in the frontend when unset ('').
ALTER TABLE lists ADD COLUMN icon TEXT NOT NULL DEFAULT '';
