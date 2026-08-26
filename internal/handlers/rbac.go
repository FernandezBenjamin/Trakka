package handlers

import (
	"errors"
	"net/http"

	"trakka/internal/db"
)

// authorizeHouseAccess writes a 403 and returns false unless the
// authenticated caller is a member (any role) of houseID.
func (app *Application) authorizeHouseAccess(w http.ResponseWriter, r *http.Request, houseID int64) bool {
	ok, err := app.DB.UserCanAccessHouse(r.Context(), userFromContext(r).ID, houseID)
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "not a member of this house")
		return false
	}
	return true
}

// authorizeHouseOwner writes a 403 and returns false unless the
// authenticated caller is the owner of houseID.
func (app *Application) authorizeHouseOwner(w http.ResponseWriter, r *http.Request, houseID int64) bool {
	role, err := app.DB.UserRoleInHouse(r.Context(), userFromContext(r).ID, houseID)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusForbidden, "not a member of this house")
		return false
	}
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	if role != "owner" {
		writeError(w, http.StatusForbidden, "only the house owner can do this")
		return false
	}
	return true
}

// authorizeItemAccess resolves listID to its house and checks membership,
// for item handlers that only have a list_id in hand (never a house_id
// directly).
func (app *Application) authorizeItemAccess(w http.ResponseWriter, r *http.Request, listID int64) bool {
	list, err := app.DB.GetList(r.Context(), listID)
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	return app.authorizeHouseAccess(w, r, list.HouseID)
}
