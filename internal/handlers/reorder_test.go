package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"trakka/internal/models"
)

// TestHandleListsReorder covers the happy path end-to-end through the
// handler (not just db.ReorderItems, already covered directly in
// internal/db/items_test.go): a House member sends the complete new
// ordering and gets back the items in that order.
func TestHandleListsReorder(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison Test", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	itemA, err := app.DB.CreateItem(ctx, list.ID, "Lait", nil, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, false)
	if err != nil {
		t.Fatalf("creating item A: %v", err)
	}
	itemB, err := app.DB.CreateItem(ctx, list.ID, "Pain", nil, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, false)
	if err != nil {
		t.Fatalf("creating item B: %v", err)
	}

	listIDStr := strconv.FormatInt(list.ID, 10)
	body := `{"item_ids":[` + strconv.FormatInt(itemB.ID, 10) + `,` + strconv.FormatInt(itemA.ID, 10) + `]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lists/"+listIDStr+"/reorder", strings.NewReader(body))
	req.SetPathValue("id", listIDStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, owner))
	rec := httptest.NewRecorder()

	app.handleListsReorder(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []models.Item
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(items) != 2 || items[0].ID != itemB.ID || items[1].ID != itemA.ID {
		t.Fatalf("unexpected order in response: %+v", items)
	}
}

// TestHandleListsReorderRequiresWriteAccess confirms a read-only List share
// (the same bar every other item-mutating handler enforces via
// authorizeListAccess's requireWrite) can't reorder items — reordering is an
// editing action on the list's contents, not something a read-only viewer
// should be able to do.
func TestHandleListsReorderRequiresWriteAccess(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	reader := mustCreateTestUser(t, app, "reader@example.com")
	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison Test", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	item, err := app.DB.CreateItem(ctx, list.ID, "Lait", nil, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, false)
	if err != nil {
		t.Fatalf("creating item: %v", err)
	}
	if _, err := app.DB.CreateOrUpdateListShare(ctx, list.ID, reader.ID, "read"); err != nil {
		t.Fatalf("sharing list read-only: %v", err)
	}

	listIDStr := strconv.FormatInt(list.ID, 10)
	body := `{"item_ids":[` + strconv.FormatInt(item.ID, 10) + `]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lists/"+listIDStr+"/reorder", strings.NewReader(body))
	req.SetPathValue("id", listIDStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, reader))
	rec := httptest.NewRecorder()

	app.handleListsReorder(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a read-only share holder, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestHandleListsReorderRejectsPartialSet confirms the handler surfaces
// db.ErrInvalidReorder as a 400 rather than a 500, and never silently
// applies a partial reorder.
func TestHandleListsReorderRejectsPartialSet(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison Test", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	itemA, err := app.DB.CreateItem(ctx, list.ID, "Lait", nil, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, false)
	if err != nil {
		t.Fatalf("creating item A: %v", err)
	}
	if _, err := app.DB.CreateItem(ctx, list.ID, "Pain", nil, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, false); err != nil {
		t.Fatalf("creating item B: %v", err)
	}

	listIDStr := strconv.FormatInt(list.ID, 10)
	body := `{"item_ids":[` + strconv.FormatInt(itemA.ID, 10) + `]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lists/"+listIDStr+"/reorder", strings.NewReader(body))
	req.SetPathValue("id", listIDStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, owner))
	rec := httptest.NewRecorder()

	app.handleListsReorder(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a partial item_ids list, got %d: %s", rec.Code, rec.Body.String())
	}
}
