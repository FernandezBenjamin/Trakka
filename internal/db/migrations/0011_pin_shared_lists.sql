-- Migration 11 ("pin shared lists to dashboard"): lets the recipient of a
-- directly-shared List choose to have it show up alongside their own
-- House's lists on the main dashboard, instead of only in the "Partagé avec
-- moi" tab. Scoped to list_shares only (not space_shares) — see
-- internal/handlers.handleListSharePin and the "Pinning shared lists"
-- bullet in CLAUDE.md for why. Defaults to 0 (not pinned): pinning is an
-- opt-in action the recipient takes per share, never automatic.
ALTER TABLE list_shares ADD COLUMN is_pinned_to_dashboard INTEGER NOT NULL DEFAULT 0;
