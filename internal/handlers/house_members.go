package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"trakka/internal/db"
)

func (app *Application) handleHouseMembersIndex(w http.ResponseWriter, r *http.Request) {
	houseID, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeHouseAccess(w, r, houseID) {
		return
	}
	members, err := app.DB.ListHouseMembers(r.Context(), houseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (app *Application) handleHouseMembersInvite(w http.ResponseWriter, r *http.Request) {
	houseID, ok := pathID(w, r)
	if !ok {
		return
	}
	if !app.authorizeHouseOwner(w, r, houseID) {
		return
	}

	var in struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	// No email-sending infrastructure exists in this project, so an invite
	// to an email with no account can never be redeemed — fail clearly
	// rather than creating a ghost membership row (which the users table's
	// CHECK constraint wouldn't even allow anyway).
	invited, err := app.DB.GetUserByEmail(r.Context(), in.Email)
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no account exists for this email yet; ask them to register or sign in, then invite them again")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}

	member, err := app.DB.AddHouseMember(r.Context(), houseID, invited.ID, "member")
	if errors.Is(err, db.ErrAlreadyMember) {
		writeError(w, http.StatusConflict, "already a member")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, member)
}

func (app *Application) handleHouseMembersRemove(w http.ResponseWriter, r *http.Request) {
	houseID, ok := pathID(w, r)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(r.PathValue("userId"), 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	// A user may always remove themselves ("leave house") regardless of
	// role; removing anyone else requires being the owner.
	caller := userFromContext(r)
	if caller.ID != userID {
		if !app.authorizeHouseOwner(w, r, houseID) {
			return
		}
	} else if !app.authorizeHouseAccess(w, r, houseID) {
		return
	}

	if err := app.DB.RemoveHouseMember(r.Context(), houseID, userID); errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not a member")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
