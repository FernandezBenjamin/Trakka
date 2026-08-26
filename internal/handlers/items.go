package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"trakka/internal/db"
	"trakka/internal/validate"
)

func (app *Application) handleItemsIndex(w http.ResponseWriter, r *http.Request) {
	listIDStr := r.URL.Query().Get("list_id")
	if listIDStr == "" {
		writeError(w, http.StatusBadRequest, "list_id query parameter is required")
		return
	}
	listID, err := strconv.ParseInt(listIDStr, 10, 64)
	if err != nil || listID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid list_id")
		return
	}

	list, err := app.DB.GetList(r.Context(), listID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "list_id does not reference an existing list")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeHouseAccess(w, r, list.HouseID) {
		return
	}

	items, err := app.DB.ListItemsByList(r.Context(), listID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (app *Application) handleItemsCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ListID      int64    `json:"list_id"`
		Title       string   `json:"title"`
		URL         string   `json:"url"`
		Quantity    int      `json:"quantity"`
		Price       *float64 `json:"price"`
		Position    int      `json:"position"`
		TargetMonth string   `json:"target_month"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Title = strings.TrimSpace(in.Title)
	if in.ListID <= 0 {
		writeError(w, http.StatusBadRequest, "list_id is required")
		return
	}
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if in.Quantity <= 0 {
		in.Quantity = 1
	}
	cleanURL, err := validate.URL(in.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Price != nil && *in.Price < 0 {
		writeError(w, http.StatusBadRequest, "price cannot be negative")
		return
	}
	cleanMonth, err := validate.Month(in.TargetMonth)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	list, err := app.DB.GetList(r.Context(), in.ListID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "list_id does not reference an existing list")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeHouseAccess(w, r, list.HouseID) {
		return
	}

	item, err := app.DB.CreateItem(r.Context(), in.ListID, in.Title, nullableString(cleanURL), in.Quantity, in.Price, false, in.Position, nullableString(cleanMonth))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	item.PriceStatus = app.scrapeProductInfo(item, "")
	writeJSON(w, http.StatusCreated, item)
}

func (app *Application) handleItemsShow(w http.ResponseWriter, r *http.Request) {
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
	if !app.authorizeItemAccess(w, r, item.ListID) {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) handleItemsUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing, err := app.DB.GetItem(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeItemAccess(w, r, existing.ListID) {
		return
	}
	previousURL := stringValue(existing.URL)

	var in struct {
		Title       string   `json:"title"`
		URL         string   `json:"url"`
		Quantity    int      `json:"quantity"`
		Price       *float64 `json:"price"`
		Done        bool     `json:"done"`
		Position    int      `json:"position"`
		TargetMonth string   `json:"target_month"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if in.Quantity <= 0 {
		in.Quantity = 1
	}
	cleanURL, err := validate.URL(in.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Price != nil && *in.Price < 0 {
		writeError(w, http.StatusBadRequest, "price cannot be negative")
		return
	}
	cleanMonth, err := validate.Month(in.TargetMonth)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// A scraped image is tied to the url it was found on: if the url just
	// changed to something new, the existing image no longer describes it
	// and must not carry over, exactly like price_auto resetting to false
	// on any full update.
	imageURL := existing.ImageURL
	if cleanURL != previousURL {
		imageURL = nil
	}

	item, err := app.DB.UpdateItem(r.Context(), id, in.Title, nullableString(cleanURL), in.Quantity, in.Price, false, imageURL, in.Done, in.Position, nullableString(cleanMonth))
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	item.PriceStatus = app.scrapeProductInfo(item, previousURL)
	writeJSON(w, http.StatusOK, item)
}

// handleItemsPatch applies a partial update (e.g. just toggling "done"),
// the common case when checking an item off a list.
func (app *Application) handleItemsPatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var in struct {
		Title       *string         `json:"title"`
		URL         *string         `json:"url"`
		Quantity    *int            `json:"quantity"`
		Price       json.RawMessage `json:"price"`
		Done        *bool           `json:"done"`
		Position    *int            `json:"position"`
		TargetMonth *string         `json:"target_month"`
	}
	if !decodeJSON(w, r, &in) {
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
	if !app.authorizeItemAccess(w, r, item.ListID) {
		return
	}
	previousURL := stringValue(item.URL)

	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		item.Title = title
	}
	if in.URL != nil {
		cleanURL, err := validate.URL(*in.URL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		newURL := nullableString(cleanURL)
		// See handleItemsUpdate: a scraped image is tied to the url it was
		// found on, so changing url invalidates it.
		if stringValue(newURL) != previousURL {
			item.ImageURL = nil
		}
		item.URL = newURL
	}
	if in.Quantity != nil {
		if *in.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "quantity must be positive")
			return
		}
		item.Quantity = *in.Quantity
	}
	// in.Price is nil when "price" was absent from the request body (leave
	// item.Price/PriceAuto untouched); present-but-"null" clears it;
	// present with a number sets it. A plain *float64 can't tell "absent"
	// apart from "explicit null" since both decode to a nil pointer, hence
	// RawMessage. Either way a price supplied in the request is a manual
	// value, so PriceAuto always resets to false here — the only path that
	// ever sets it true is the scraper's own UpdateItemPriceIfMissing.
	if in.Price != nil {
		if string(in.Price) == "null" {
			item.Price = nil
			item.PriceAuto = false
		} else {
			var price float64
			if err := json.Unmarshal(in.Price, &price); err != nil {
				writeError(w, http.StatusBadRequest, "price must be a number")
				return
			}
			if price < 0 {
				writeError(w, http.StatusBadRequest, "price cannot be negative")
				return
			}
			item.Price = &price
			item.PriceAuto = false
		}
	}
	if in.Done != nil {
		item.Done = *in.Done
	}
	if in.Position != nil {
		item.Position = *in.Position
	}
	// A nil in.TargetMonth means the field was absent from the request
	// (leave item.TargetMonth untouched, e.g. a plain "done" toggle);
	// present-but-empty clears it back to unscheduled, mirroring how URL is
	// handled above.
	if in.TargetMonth != nil {
		cleanMonth, err := validate.Month(*in.TargetMonth)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		item.TargetMonth = nullableString(cleanMonth)
	}

	updated, err := app.DB.UpdateItem(r.Context(), id, item.Title, item.URL, item.Quantity, item.Price, item.PriceAuto, item.ImageURL, item.Done, item.Position, item.TargetMonth)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	updated.PriceStatus = app.scrapeProductInfo(updated, previousURL)
	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) handleItemsDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing, err := app.DB.GetItem(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeItemAccess(w, r, existing.ListID) {
		return
	}

	if err := app.DB.DeleteItem(r.Context(), id); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
