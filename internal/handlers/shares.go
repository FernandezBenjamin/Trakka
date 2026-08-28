package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"trakka/internal/db"
	"trakka/internal/models"
)

// authorizeSpaceOwner writes a 404 (never a 403 — matches
// GetCustomCategoryForUser's own "don't let a lookup confirm another user's
// data exists" convention, shared with handleCustomCategoriesUpdate/Delete)
// and returns false unless the caller owns categoryID. Sharing a Space is
// the owning user's call alone, the same as renaming/deleting it.
func (app *Application) authorizeSpaceOwner(w http.ResponseWriter, r *http.Request, categoryID int64) bool {
	if _, err := app.DB.GetCustomCategoryForUser(r.Context(), categoryID, userFromContext(r).ID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "custom category not found")
		return false
	} else if err != nil {
		app.serverError(w, r, err)
		return false
	}
	return true
}

// handleSpaceShareIndex lists everyone a Space has been shared with —
// the roster shown in the share modal.
func (app *Application) handleSpaceShareIndex(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeSpaceOwner(w, r, id) {
		return
	}

	shares, err := app.DB.ListSpaceShares(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, shares)
}

// handleSpaceShareCreate grants (or updates the permission of) a Space
// share, looked up by the recipient's email — mirrors
// handleHouseMembersInvite's "no email-sending infrastructure, so fail
// clearly rather than creating a ghost row" reasoning.
func (app *Application) handleSpaceShareCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeSpaceOwner(w, r, id) {
		return
	}

	var in struct {
		Email      string `json:"email"`
		Permission string `json:"permission"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if !models.ValidSharePermissions[in.Permission] {
		writeError(w, http.StatusBadRequest, "permission must be \"read\" or \"write\"")
		return
	}

	target, err := app.DB.GetUserByEmail(r.Context(), in.Email)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no account exists for this email yet; ask them to register or sign in, then share again")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if target.ID == userFromContext(r).ID {
		writeError(w, http.StatusBadRequest, "cannot share a space with yourself")
		return
	}

	share, err := app.DB.CreateOrUpdateSpaceShare(r.Context(), id, target.ID, in.Permission)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, share)
}

func (app *Application) handleSpaceShareRevoke(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeSpaceOwner(w, r, id) {
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := app.DB.RevokeSpaceShare(r.Context(), id, userID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "share not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListShareIndex lists everyone a List has been shared with. Scoped
// to actual House membership (not authorizeListAccess) — see
// handleListShareCreate for why: only a list's real House can manage who
// else it's shared with, so access granted through a share can never be
// used to extend further access.
func (app *Application) handleListShareIndex(w http.ResponseWriter, r *http.Request) {
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

	shares, err := app.DB.ListListShares(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, shares)
}

// handleListShareCreate grants (or updates the permission of) a List
// share, looked up by the recipient's email. Deliberately requires actual
// House membership of the list (authorizeHouseAccess), not merely write
// access to it (authorizeListAccess) — a list has no single "owner" the way
// a Space does, so letting anyone with write-via-share also grant further
// shares would let access cascade uncontrolled; only the list's real House
// members, who fundamentally own it, can extend it to someone new.
func (app *Application) handleListShareCreate(w http.ResponseWriter, r *http.Request) {
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

	var in struct {
		Email      string `json:"email"`
		Permission string `json:"permission"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if !models.ValidSharePermissions[in.Permission] {
		writeError(w, http.StatusBadRequest, "permission must be \"read\" or \"write\"")
		return
	}

	target, err := app.DB.GetUserByEmail(r.Context(), in.Email)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no account exists for this email yet; ask them to register or sign in, then share again")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	if target.ID == userFromContext(r).ID {
		writeError(w, http.StatusBadRequest, "cannot share a list with yourself")
		return
	}
	alreadyMember, err := app.DB.UserCanAccessHouse(r.Context(), target.ID, list.HouseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if alreadyMember {
		writeError(w, http.StatusBadRequest, "this person is already a member of the list's house")
		return
	}

	share, err := app.DB.CreateOrUpdateListShare(r.Context(), id, target.ID, in.Permission)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, share)
}

func (app *Application) handleListShareRevoke(w http.ResponseWriter, r *http.Request) {
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
	userID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := app.DB.RevokeListShare(r.Context(), id, userID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "share not found")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
