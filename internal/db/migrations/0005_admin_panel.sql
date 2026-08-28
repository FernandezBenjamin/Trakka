-- Migration 5 ("Add admin panel"): users.is_admin grants access to
-- /api/v1/admin/... (set automatically for the very first account ever
-- created, never settable through the registration/profile API — see
-- internal/db.CreateUser). system_settings is a generic key/value store for
-- runtime settings (OIDC config, open/closed registration, instance name)
-- an admin can change without a server restart; a key with a row here
-- always overrides its equivalent environment variable — see
-- internal/settings.Resolve.
ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;

CREATE TABLE system_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
