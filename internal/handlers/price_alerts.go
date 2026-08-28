package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"trakka/internal/db"
	"trakka/internal/models"
	"trakka/internal/scraper"
)

// priceCheckTimeout bounds a single item's fetch during the periodic
// price-drop scan or an on-demand check — long enough for a normal product
// page, short enough that one slow/unresponsive site can't stall the whole
// scan (or an on-demand request a user is actively waiting on)
// indefinitely.
const priceCheckTimeout = 10 * time.Second

// checkItemForBetterPrice re-fetches item's product page and records a
// price_alerts row (via db.CreatePriceAlertIfNonePending) if a lower price
// than the item's current one is found there. It is the single entry point
// both RunPriceAlertScan (periodic, all eligible items) and
// handleItemsPriceCheck (on-demand, one item) call. Every failure mode
// (network error, timeout, blocked host, nothing recognizable on the page,
// or simply no price lower than what's already known) is treated
// identically as "nothing to alert on" — mirroring
// internal/scraper.FetchProductInfo's own contract — never as an error
// worth surfacing, since this is a best-effort background check.
func (app *Application) checkItemForBetterPrice(ctx context.Context, item *models.Item) error {
	if item.URL == nil || *item.URL == "" || item.Price == nil {
		return nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, priceCheckTimeout)
	defer cancel()

	info, err := scraper.FetchProductInfo(fetchCtx, *item.URL)
	if err != nil {
		app.Logger.Debug("price check found nothing", "item_id", item.ID, "url", *item.URL, "error", err)
		return nil
	}
	if info.Price == nil || *info.Price >= *item.Price {
		return nil
	}

	if err := app.DB.CreatePriceAlertIfNonePending(ctx, item.ID, *item.Price, *info.Price, *item.URL); err != nil {
		return fmt.Errorf("recording price alert for item %d: %w", item.ID, err)
	}
	return nil
}

// RunPriceAlertScan checks every eligible item (see
// db.ListItemsForPriceScan — not done, has both a url and a price) for a
// better price, creating a pending price_alerts row for each drop found.
// Called on a timer from cmd/server/main.go; also safe to call directly for
// an immediate, whole-catalog on-demand scan. Best-effort throughout: one
// item's check failing (network error, or a genuine internal/db error) is
// logged and never stops the rest of the scan.
func (app *Application) RunPriceAlertScan(ctx context.Context) {
	items, err := app.DB.ListItemsForPriceScan(ctx)
	if err != nil {
		app.Logger.Error("listing items for price alert scan", "error", err)
		return
	}

	app.Logger.Info("running price alert scan", "item_count", len(items))
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		if err := app.checkItemForBetterPrice(ctx, item); err != nil {
			app.Logger.Error("price alert scan check failed", "item_id", item.ID, "error", err)
		}
	}
}

// handlePriceAlertsIndex lists price alerts for a house, filterable by
// status — the notification bell calls this with ?status=pending. house_id
// is required, mirroring handleItemsIndex's ?list_id requirement, since a
// price alert only makes sense scoped to a house the caller is a member of.
func (app *Application) handlePriceAlertsIndex(w http.ResponseWriter, r *http.Request) {
	houseIDStr := r.URL.Query().Get("house_id")
	if houseIDStr == "" {
		writeError(w, http.StatusBadRequest, "house_id query parameter is required")
		return
	}
	houseID, err := strconv.ParseInt(houseIDStr, 10, 64)
	if err != nil || houseID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid house_id")
		return
	}
	if !app.authorizeHouseAccess(w, r, houseID) {
		return
	}

	status := r.URL.Query().Get("status")
	if status != "" && !models.ValidPriceAlertStatuses[status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	alerts, err := app.DB.ListPriceAlertsByHouse(r.Context(), houseID, status)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// handlePriceAlertsUpdate resolves a pending alert: {"status": "accepted"}
// applies its found_price to the item, {"status": "rejected"} just
// dismisses it. Once resolved an alert can never be re-actioned — see
// db.AcceptPriceAlert/RejectPriceAlert's shared "WHERE status = 'pending'"
// guard — so a repeat call (e.g. a double click) reports 409 rather than
// silently doing nothing or erroring as "not found".
func (app *Application) handlePriceAlertsUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var in struct {
		Status string `json:"status"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Status != "accepted" && in.Status != "rejected" {
		writeError(w, http.StatusBadRequest, "status must be \"accepted\" or \"rejected\"")
		return
	}

	alert, err := app.DB.GetPriceAlert(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "price alert not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeItemAccess(w, r, alert.ListID, true) {
		return
	}

	var updated *models.PriceAlert
	if in.Status == "accepted" {
		updated, err = app.DB.AcceptPriceAlert(r.Context(), id)
	} else {
		updated, err = app.DB.RejectPriceAlert(r.Context(), id)
	}
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusConflict, "price alert was already resolved")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleItemsPriceCheck triggers an immediate, synchronous price check for
// one item (the "à la demande" counterpart to RunPriceAlertScan's periodic
// sweep) and reports whatever pending alert exists for it afterward — a
// freshly created one, one from an earlier check that's still pending, or
// null if nothing better than the item's current price is known. Bounded
// by priceCheckTimeout, same as a single item within the periodic scan;
// long enough for a normal product page, short enough that this explicit,
// user-triggered action still responds promptly.
func (app *Application) handleItemsPriceCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	item, err := app.DB.GetItem(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeItemAccess(w, r, item.ListID, true) {
		return
	}
	if item.URL == nil || *item.URL == "" {
		writeError(w, http.StatusBadRequest, "item has no url to check")
		return
	}
	if item.Price == nil {
		writeError(w, http.StatusBadRequest, "item has no price to compare against")
		return
	}

	if err := app.checkItemForBetterPrice(r.Context(), item); err != nil {
		app.serverError(w, r, err)
		return
	}

	alert, err := app.DB.GetPendingPriceAlertForItem(r.Context(), item.ID)
	if errors.Is(err, db.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"alert": nil})
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alert": alert})
}
