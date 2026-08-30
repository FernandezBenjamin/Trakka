-- Migration 12 ("pin shared spaces to dashboard"): extends the pinning
-- mechanism introduced for a directly-shared List (migration 11) to a
-- whole shared Space. Pinning a Space lets its recipient surface every list
-- reachable through it (present and future — no per-list action needed) on
-- their own dashboard/Espaces tab in one action, instead of having to pin
-- each of that Space's lists individually — see
-- internal/handlers.handleSpaceSharePin and the "Pinning shared spaces to
-- the dashboard" bullet in CLAUDE.md. Defaults to 0 (not pinned): same
-- opt-in-per-share convention as list_shares.is_pinned_to_dashboard.
ALTER TABLE space_shares ADD COLUMN is_pinned_to_dashboard INTEGER NOT NULL DEFAULT 0;
