package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"trakka/internal/db"
	"trakka/internal/models"
)

// handleListsIndex lists the caller's lists. ?shared_with_me=true switches
// to a different, mutually exclusive mode: every List reachable only via a
// list_shares/space_shares grant (see db.ListSharedListsForUser) rather than
// the caller's own House-scoped lists — house_id/type filters don't apply
// to that mode, mirroring how ?house_id= itself is optional below.
func (app *Application) handleListsIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("shared_with_me") == "true" {
		lists, err := app.DB.ListSharedListsForUser(r.Context(), userFromContext(r).ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, lists)
		return
	}

	typeFilter := r.URL.Query().Get("type")
	if typeFilter != "" && !models.ValidListTypes[typeFilter] {
		writeError(w, http.StatusBadRequest, "invalid type filter")
		return
	}

	var houseID int64
	if houseIDStr := r.URL.Query().Get("house_id"); houseIDStr != "" {
		var err error
		houseID, err = strconv.ParseInt(houseIDStr, 10, 64)
		if err != nil || houseID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid house_id")
			return
		}
		if !app.authorizeHouseAccess(w, r, houseID) {
			return
		}
	}

	lists, err := app.DB.ListListsForUser(r.Context(), userFromContext(r).ID, typeFilter, houseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lists)
}

func (app *Application) handleListsCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name             string `json:"name"`
		Type             string `json:"type"`
		HouseID          int64  `json:"house_id"`
		CustomCategoryID *int64 `json:"custom_category_id"`
		Icon             string `json:"icon"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if in.Type == "" {
		in.Type = "shopping"
	}
	if !models.ValidListTypes[in.Type] {
		writeError(w, http.StatusBadRequest, "type must be one of 'todo', 'shopping', 'groceries', 'recurring_shopping', 'custom'")
		return
	}
	in.Icon = strings.TrimSpace(in.Icon)
	if in.HouseID <= 0 {
		writeError(w, http.StatusBadRequest, "house_id is required")
		return
	}
	if _, err := app.DB.GetHouse(r.Context(), in.HouseID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "house_id does not reference an existing house")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeHouseAccess(w, r, in.HouseID) {
		return
	}
	if !app.validateCustomCategoryOwnership(w, r, in.CustomCategoryID) {
		return
	}

	list, err := app.DB.CreateList(r.Context(), in.Name, in.Type, in.HouseID, in.CustomCategoryID, in.Icon)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

// validateCustomCategoryOwnership writes a 400 and returns false if id is
// non-nil but doesn't reference a category owned by the caller (or is
// non-positive). A nil id (meaning "leave unattached"/"dissociate") is
// always valid and returns true without a lookup.
func (app *Application) validateCustomCategoryOwnership(w http.ResponseWriter, r *http.Request, id *int64) bool {
	if id == nil {
		return true
	}
	if *id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid custom_category_id")
		return false
	}
	if _, err := app.DB.GetCustomCategoryForUser(r.Context(), *id, userFromContext(r).ID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "custom_category_id does not reference a category you own")
		return false
	} else if err != nil {
		app.serverError(w, r, err)
		return false
	}
	return true
}

func (app *Application) handleListsShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	list, err := app.DB.GetList(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !app.authorizeListAccess(w, r, list, false) {
		return
	}

	items, err := app.DB.ListItemsByList(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	list.Items = items

	writeJSON(w, http.StatusOK, list)
}

func (app *Application) handleListsUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing, err := app.DB.GetList(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Renaming/retyping/recategorizing a list counts as "editing" it, so a
	// write-level List/Space share is enough here — unlike deleting it
	// outright (handleListsDelete below), which stays House-membership-only.
	if !app.authorizeListAccess(w, r, existing, true) {
		return
	}

	var in struct {
		Name             string `json:"name"`
		Type             string `json:"type"`
		CustomCategoryID *int64 `json:"custom_category_id"`
		Icon             string `json:"icon"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if in.Type == "" {
		in.Type = "shopping"
	}
	if !models.ValidListTypes[in.Type] {
		writeError(w, http.StatusBadRequest, "type must be one of 'todo', 'shopping', 'groceries', 'recurring_shopping', 'custom'")
		return
	}
	in.Icon = strings.TrimSpace(in.Icon)
	if !app.validateCustomCategoryOwnership(w, r, in.CustomCategoryID) {
		return
	}

	list, err := app.DB.UpdateList(r.Context(), id, in.Name, in.Type, in.CustomCategoryID, in.Icon)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (app *Application) handleListsDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing, err := app.DB.GetList(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Deliberately House-membership-only, unlike handleListsUpdate above: a
	// write-level List/Space share lets someone edit a list and its items,
	// not destroy the list itself (and everyone else's items in it).
	if !app.authorizeHouseAccess(w, r, existing.HouseID) {
		return
	}

	if err := app.DB.DeleteList(r.Context(), id); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "list not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
