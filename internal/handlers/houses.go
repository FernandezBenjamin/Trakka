package handlers

import (
	"errors"
	"net/http"

	"trakka/internal/db"
	"trakka/internal/validate"
)

// houseResponse adds the caller's own role to a house, computed per
// request rather than stored, so the frontend can conditionally show
// owner-only controls (rename, delete, manage members).
type houseResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	Role      string `json:"role"`
}

func (app *Application) withRole(w http.ResponseWriter, r *http.Request, houseID int64) (string, bool) {
	role, err := app.DB.UserRoleInHouse(r.Context(), userFromContext(r).ID, houseID)
	if err != nil {
		app.serverError(w, r, err)
		return "", false
	}
	return role, true
}

func (app *Application) handleHousesIndex(w http.ResponseWriter, r *http.Request) {
	userID := userFromContext(r).ID
	houses, err := app.DB.ListHousesForUser(r.Context(), userID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	out := make([]houseResponse, 0, len(houses))
	for _, h := range houses {
		role, ok := app.withRole(w, r, h.ID)
		if !ok {
			return
		}
		out = append(out, houseResponse{ID: h.ID, Name: h.Name, CreatedAt: h.CreatedAt, Role: role})
	}
	writeJSON(w, http.StatusOK, out)
}

func (app *Application) handleHousesCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Name = validate.Text(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validate.MaxLen(in.Name, validate.MaxNameLen) {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}

	house, err := app.DB.CreateHouseWithOwner(r.Context(), in.Name, userFromContext(r).ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, houseResponse{ID: house.ID, Name: house.Name, CreatedAt: house.CreatedAt, Role: "owner"})
}

func (app *Application) handleHousesShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeHouseAccess(w, r, id) {
		return
	}

	house, err := app.DB.GetHouse(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "house not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	role, ok := app.withRole(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, houseResponse{ID: house.ID, Name: house.Name, CreatedAt: house.CreatedAt, Role: role})
}

func (app *Application) handleHousesUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeHouseOwner(w, r, id) {
		return
	}

	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}

	in.Name = validate.Text(in.Name)
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validate.MaxLen(in.Name, validate.MaxNameLen) {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}

	house, err := app.DB.UpdateHouse(r.Context(), id, in.Name)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "house not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, houseResponse{ID: house.ID, Name: house.Name, CreatedAt: house.CreatedAt, Role: "owner"})
}

func (app *Application) handleHousesDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeHouseOwner(w, r, id) {
		return
	}

	err := app.DB.DeleteHouse(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "house not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
