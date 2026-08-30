package handlers

import (
	"errors"
	"net/http"

	"trakka/internal/db"
	"trakka/internal/models"
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

// authorizeAdmin writes a 403 and returns false unless the authenticated
// caller has the system-wide admin role (see models.User.IsAdmin) — the
// gate for every /api/v1/admin/... endpoint.
func (app *Application) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !userFromContext(r).IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}

// authorizeListAccess writes a 403 and returns false unless the
// authenticated caller has at least read access to list — House membership
// (any role), OR the list's parent Space being shared with them
// (space_shares), OR the list itself being shared with them directly
// (list_shares); see db.AccessLevelForList. requireWrite additionally
// demands "write"-level access rather than merely "read", for handlers that
// mutate the list or its items rather than just reading them.
func (app *Application) authorizeListAccess(w http.ResponseWriter, r *http.Request, list *models.List, requireWrite bool) bool {
	level, err := app.DB.AccessLevelForList(r.Context(), userFromContext(r).ID, list)
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	if level == "" {
		writeError(w, http.StatusForbidden, "no access to this list")
		return false
	}
	if requireWrite && level != "write" {
		writeError(w, http.StatusForbidden, "read-only access to this list")
		return false
	}
	return true
}

// authorizeItemAccess resolves listID to its list and applies
// authorizeListAccess, for item handlers that only have a list_id in hand
// (never the list itself directly).
func (app *Application) authorizeItemAccess(w http.ResponseWriter, r *http.Request, listID int64, requireWrite bool) bool {
	list, err := app.DB.GetList(r.Context(), listID)
	if errors.Is(err, db.ErrNotFound) {
		// A row pointing at a list that no longer exists is a missing
		// resource, not a server fault: reporting it as a 500 both misleads
		// the client and puts a spurious error in the operator's logs.
		writeError(w, http.StatusNotFound, "list not found")
		return false
	}
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	return app.authorizeListAccess(w, r, list, requireWrite)
}
