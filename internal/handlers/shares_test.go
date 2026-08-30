package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"trakka/internal/db"
	"trakka/internal/models"
)

// newTestApplication opens a throwaway on-disk SQLite database (same
// pattern as internal/db's own openTestDB helper) and wraps it in a bare
// *Application — enough to call a handler method directly without going
// through Routes()'s full middleware chain, which needs a live session
// cookie/auth service this test has no use for.
func newTestApplication(t *testing.T) *Application {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	d, err := db.Open(filepath.Join(t.TempDir(), "trakka.db"), logger)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return &Application{DB: d, Logger: logger}
}

func mustCreateTestUser(t *testing.T, app *Application, email string) *models.User {
	t.Helper()
	hash := "x"
	user, err := app.DB.CreateUser(context.Background(), email, &hash, nil, nil, email)
	if err != nil {
		t.Fatalf("creating user %s: %v", email, err)
	}
	return user
}

// TestHandleListSharePinDoesNotRequireHouseMembership is a regression guard
// for a bug report claiming PATCH /api/v1/lists/{id}/share/pin rejected a
// share recipient with "not a member of this house": the whole premise of
// pinning a shared list is that the recipient is, in the ordinary case,
// NOT a member of the list's House — that's exactly why the list had to be
// shared with them directly rather than them just seeing it through House
// membership. Authorization for this endpoint must be "does the caller
// hold a list_shares row for this list" (SetListSharePinned's own WHERE
// clause), never authorizeHouseAccess — this test calls
// handleListSharePin directly (bypassing Routes()'s mux/middleware, which
// isn't what's under test here) with a caller who has zero relationship to
// the list's House at all, and asserts it succeeds.
func TestHandleListSharePinDoesNotRequireHouseMembership(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	external := mustCreateTestUser(t, app, "external@example.com")

	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison du owner", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Liste privée", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}

	// Sanity check: `external` really has no House relationship at all — if
	// this ever stopped being true the rest of the test would prove
	// nothing about the bug it guards against.
	canAccess, err := app.DB.UserCanAccessHouse(ctx, external.ID, house.ID)
	if err != nil {
		t.Fatalf("UserCanAccessHouse: %v", err)
	}
	if canAccess {
		t.Fatalf("test setup bug: external unexpectedly has access to owner's House")
	}

	if _, err := app.DB.CreateOrUpdateListShare(ctx, list.ID, external.ID, "read"); err != nil {
		t.Fatalf("sharing list with external: %v", err)
	}

	listIDStr := strconv.FormatInt(list.ID, 10)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/lists/"+listIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	req.SetPathValue("id", listIDStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, external))
	rec := httptest.NewRecorder()

	app.handleListSharePin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 pinning a list shared directly with a non-House-member, got %d: %s", rec.Code, rec.Body.String())
	}

	var share models.ListShare
	if err := json.Unmarshal(rec.Body.Bytes(), &share); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !share.IsPinnedToDashboard {
		t.Fatalf("expected the response to reflect the pinned state, got %+v", share)
	}

	// And the House owner — who has full House membership but no
	// list_shares row of their own on their own list — must NOT be able to
	// pin it: house membership must never substitute for holding a share.
	ownerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/lists/"+listIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	ownerReq.SetPathValue("id", listIDStr)
	ownerReq = ownerReq.WithContext(context.WithValue(ownerReq.Context(), userContextKey, owner))
	ownerRec := httptest.NewRecorder()

	app.handleListSharePin(ownerRec, ownerReq)

	if ownerRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for the House owner (no list_shares row of their own), got %d: %s", ownerRec.Code, ownerRec.Body.String())
	}
}
