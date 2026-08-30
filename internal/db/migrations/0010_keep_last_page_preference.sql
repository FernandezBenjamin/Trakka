-- Migration 10 ("keep last page on launch"): a per-user preference letting
-- someone opt out of Trakka reopening on whatever dashboard tab or list
-- they had open last, in favor of always landing back on the dashboard.
-- Defaults to enabled (1) to match the feature's own "on by default" spec
-- and the frontend's fallback when this hasn't been fetched from the
-- server yet (see static/js/settings.js).
ALTER TABLE users ADD COLUMN keep_last_page INTEGER NOT NULL DEFAULT 1;
