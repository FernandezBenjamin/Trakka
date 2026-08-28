package handlers

import (
	"errors"
	"net/http"
	"strings"

	"trakka/internal/db"
	"trakka/internal/validate"
)

// handleCustomCategoriesIndex lists the caller's own custom categories —
// unlike houses/lists, a category has no shared "membership": it's always
// scoped to the authenticated user, never to a house.
func (app *Application) handleCustomCategoriesIndex(w http.ResponseWriter, r *http.Request) {
	categories, err := app.DB.ListCustomCategoriesForUser(r.Context(), userFromContext(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (app *Application) handleCustomCategoriesCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
		Position int    `json:"position"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	in.Icon = strings.TrimSpace(in.Icon)
	cleanColor, err := validate.Color(in.Color)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	category, err := app.DB.CreateCustomCategory(r.Context(), userFromContext(r).ID, in.Name, in.Icon, cleanColor, in.Position)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, category)
}

// handleCustomCategoriesUpdate is a full replace, scoped to the caller's
// own categories — db.UpdateCustomCategoryForUser's WHERE ... AND user_id
// guard is what makes a category owned by someone else come back as
// ErrNotFound here rather than succeeding or leaking a 403 that would
// confirm its existence.
func (app *Application) handleCustomCategoriesUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var in struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
		Position int    `json:"position"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	in.Icon = strings.TrimSpace(in.Icon)
	cleanColor, err := validate.Color(in.Color)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	category, err := app.DB.UpdateCustomCategoryForUser(r.Context(), id, userFromContext(r).ID, in.Name, in.Icon, cleanColor, in.Position)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "custom category not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, category)
}

func (app *Application) handleCustomCategoriesDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if err := app.DB.DeleteCustomCategoryForUser(r.Context(), id, userFromContext(r).ID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "custom category not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
