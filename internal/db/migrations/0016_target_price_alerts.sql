-- Migration 16 ("Price drop alerts"): lets a user set a target price on an
-- item and be notified (in-app toast + push, see
-- internal/handlers.checkPriceDropAlert) once its price drops to or below
-- that threshold, whether the price changed manually or via the background
-- URL scraper. target_price is nullable (unset means no threshold);
-- alert_on_price_drop is a plain opt-in flag, independent of target_price
-- itself being set, mirroring how is_urgent (migration 3) is a plain
-- user-set toggle alongside the fields it doesn't derive from.
ALTER TABLE items ADD COLUMN target_price REAL;
ALTER TABLE items ADD COLUMN alert_on_price_drop INTEGER NOT NULL DEFAULT 0;
