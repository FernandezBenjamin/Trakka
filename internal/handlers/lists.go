package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"trakka/internal/db"
	"trakka/internal/models"
)

func (app *Application) handleListsIndex(w http.ResponseWriter, r *http.Request) {
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
		Name    string `json:"name"`
		Type    string `json:"type"`
		HouseID int64  `json:"house_id"`
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

	list, err := app.DB.CreateList(r.Context(), in.Name, in.Type, in.HouseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, list)
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
	if !app.authorizeHouseAccess(w, r, list.HouseID) {
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
	if !app.authorizeHouseAccess(w, r, existing.HouseID) {
		return
	}

	var in struct {
		Name string `json:"name"`
		Type string `json:"type"`
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

	list, err := app.DB.UpdateList(r.Context(), id, in.Name, in.Type)
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
