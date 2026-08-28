-- Migration 4 ("Track price for object with link to search if item exist
-- with lower price exist"): price-drop / better-deal alerts surfaced by
-- internal/scraper's periodic and on-demand price checks (see
-- internal/handlers/price_alerts.go). A pending alert means a lower price
-- than the item's current price was found at source_url; accepting one
-- applies found_price to the item, rejecting one just dismisses it.
-- original_price is a snapshot taken when the alert was created, not
-- re-read from the item later, so a notification always reflects what the
-- comparison was actually made against.
CREATE TABLE price_alerts (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id        INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    original_price REAL NOT NULL,
    found_price    REAL NOT NULL,
    source_url     TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_price_alerts_item_id ON price_alerts(item_id);
CREATE INDEX idx_price_alerts_status ON price_alerts(status);
