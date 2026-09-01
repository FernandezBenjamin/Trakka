package handlers

import (
	"errors"
	"net/http"

	"trakka/internal/db"
)

// handleListsReorder applies a manual drag-and-drop reordering of a list's
// items: the request carries the *complete* new ordering as item_ids, and
// db.ReorderItems assigns each one a position matching its index. Requiring
// the complete set (rather than accepting a partial reorder) is what makes
// the resulting position values unambiguous — see db.ReorderItems's comment.
// Write-level list access is enough here, the same bar handleItemsUpdate/
// handleItemsPatch already use for editing an item, since rearranging items
// is an editing action on the list's contents, not a house-management-level
// one like deleting the list itself.
func (app *Application) handleListsReorder(w http.ResponseWriter, r *http.Request) {
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
	if !app.authorizeListAccess(w, r, list, true) {
		return
	}

	var in struct {
		ItemIDs []int64 `json:"item_ids"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if len(in.ItemIDs) == 0 {
		writeError(w, http.StatusBadRequest, "item_ids is required")
		return
	}

	items, err := app.DB.ReorderItems(r.Context(), id, in.ItemIDs)
	if errors.Is(err, db.ErrInvalidReorder) {
		writeError(w, http.StatusBadRequest, "item_ids must list every item currently in this list exactly once")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}
