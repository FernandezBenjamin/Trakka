package handlers

import (
	"context"
	"time"

	"trakka/internal/models"
	"trakka/internal/scraper"
)

// targetPriceScanFetchTimeout bounds a single item's re-scrape during the
// periodic target-price worker — the same bound priceCheckTimeout applies to
// the separate price_alerts scan, and for the same reason: long enough for a
// normal product page, short enough that one slow/unresponsive site can't
// stall the whole run.
const targetPriceScanFetchTimeout = 10 * time.Second

// targetPriceScanDelay is the pause between two consecutive items' fetches
// within a single RunTargetPriceScan pass — deliberate rate limiting so a
// house tracking many items doesn't hit a merchant's site (Amazon in
// particular) with a burst of near-simultaneous requests, which risks the
// server's IP getting throttled or blocked outright. This is a politeness
// delay between requests to hosts this app doesn't control, unrelated to
// internal/scraper's own fetchTimeout (the bound on a single request).
const targetPriceScanDelay = 5 * time.Second

// RunTargetPriceScan re-scrapes every item with an active price-drop
// threshold (db.ListItemsForTargetPriceScan: alert_on_price_drop = true, a
// url, and a target_price set) and applies whatever current price it finds,
// notifying via checkPriceDropAlert on a false→true transition exactly as a
// manual price edit already does through the item handlers. Called on a
// timer from cmd/server/main.go (runTargetPriceScanLoop); also safe to call
// directly for an immediate, whole-catalog on-demand scan. Best-effort
// throughout: one item's check failing (network error, or a genuine
// internal/db error) is logged and never stops the rest of the scan.
// targetPriceScanDelay is paused between items (not after the last one), so
// a full pass takes at least len(items) * ~5s — a large catalog should lean
// on a longer SCRAPE_INTERVAL rather than a shorter per-item delay.
func (app *Application) RunTargetPriceScan(ctx context.Context) {
	items, err := app.DB.ListItemsForTargetPriceScan(ctx)
	if err != nil {
		app.Logger.Error("listing items for target price scan", "error", err)
		return
	}

	app.Logger.Info("running target price scan", "item_count", len(items))
	for i, item := range items {
		if ctx.Err() != nil {
			return
		}
		if err := app.rescanItemTargetPrice(ctx, item); err != nil {
			app.Logger.Error("target price scan check failed", "item_id", item.ID, "error", err)
		}
		if i < len(items)-1 && sleepOrCanceled(ctx, targetPriceScanDelay) {
			return
		}
	}
}

// rescanItemTargetPrice re-fetches item's product page, applies whatever
// price it finds via db.UpdateItemPriceFromScan (a compare-and-swap against
// the price/url this call started with, so a concurrent edit elsewhere can
// never be clobbered by a slow scrape), and fires checkPriceDropAlert on a
// false→true transition of item's own alert_on_price_drop/target_price
// condition. A page with no recognizable price, no change from the item's
// current price, a network error, or losing the compare-and-swap race are
// all treated identically as "nothing to do" — the same best-effort
// contract every other scraper-driven code path in this package follows.
func (app *Application) rescanItemTargetPrice(ctx context.Context, item *models.Item) error {
	if item.URL == nil || *item.URL == "" {
		return nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, targetPriceScanFetchTimeout)
	defer cancel()

	info, err := scraper.FetchProductInfo(fetchCtx, *item.URL, app.Logger)
	if err != nil {
		app.Logger.Debug("target price scan found nothing", "item_id", item.ID, "url", *item.URL, "error", err)
		return nil
	}
	if info.Price == nil {
		return nil
	}
	newPrice := *info.Price
	if item.Price != nil && *item.Price == newPrice {
		return nil
	}

	// Captured from item's state as it was read at the start of this scan
	// pass, before the price below is applied — the same "compute wasActive
	// before the change lands" pattern checkPriceDropAlert's own doc comment
	// requires of every caller.
	wasActive := priceAlertCondition(item)

	applied, err := app.DB.UpdateItemPriceFromScan(ctx, item.ID, *item.URL, item.Price, newPrice)
	if err != nil {
		return err
	}
	if !applied {
		return nil
	}

	updated := *item
	updated.Price = &newPrice
	updated.PriceAuto = true
	app.checkPriceDropAlert(&updated, wasActive)
	return nil
}

// sleepOrCanceled pauses for d, returning true the moment ctx is canceled
// instead of waiting out the rest of d — the shared cancelable-sleep helper
// for the pacing delay between two consecutive items within a scan, so a
// slow multi-item scan still responds to shutdown promptly rather than
// finishing its current delay first.
func sleepOrCanceled(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}
