package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

// TestHandleSpaceSharePinDoesNotRequireOwnership is the Space-level
// equivalent of TestHandleListSharePinDoesNotRequireHouseMembership: the
// recipient of a Space share (not its owner) must be able to pin it, and
// the Space's own owner — who by definition holds no space_shares row on
// their own category — must get a 404 rather than being let through some
// ownership-based shortcut.
func TestHandleSpaceSharePinDoesNotRequireOwnership(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	recipient := mustCreateTestUser(t, app, "recipient@example.com")

	category, err := app.DB.CreateCustomCategory(ctx, owner.ID, "Vacances", "🏖️", "", 0)
	if err != nil {
		t.Fatalf("creating category: %v", err)
	}
	if _, err := app.DB.CreateOrUpdateSpaceShare(ctx, category.ID, recipient.ID, "read"); err != nil {
		t.Fatalf("sharing space with recipient: %v", err)
	}

	categoryIDStr := strconv.FormatInt(category.ID, 10)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/custom-categories/"+categoryIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	req.SetPathValue("id", categoryIDStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, recipient))
	rec := httptest.NewRecorder()

	app.handleSpaceSharePin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 pinning a space shared with the recipient, got %d: %s", rec.Code, rec.Body.String())
	}

	var share models.SpaceShare
	if err := json.Unmarshal(rec.Body.Bytes(), &share); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !share.IsPinnedToDashboard {
		t.Fatalf("expected the response to reflect the pinned state, got %+v", share)
	}

	ownerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/custom-categories/"+categoryIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	ownerReq.SetPathValue("id", categoryIDStr)
	ownerReq = ownerReq.WithContext(context.WithValue(ownerReq.Context(), userContextKey, owner))
	ownerRec := httptest.NewRecorder()

	app.handleSpaceSharePin(ownerRec, ownerReq)

	if ownerRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for the Space's own owner (no space_shares row of their own), got %d: %s", ownerRec.Code, ownerRec.Body.String())
	}
}

// TestHandleSpaceSharePinFallsBackToHouseMembership is a regression guard
// for the second half of this session's bug report: a fellow House member
// with no space_shares row at all — nobody explicitly shared the Space with
// them, they can just see it because a fellow member tagged one of the
// House's own lists with it — must still be able to pin it via this
// endpoint. handleSpaceSharePin must fall back to SetSpaceHousePinned once
// SetSpaceSharePinned reports ErrNotFound, and the response in that case is
// a models.SpacePinStatus (not a models.SpaceShare, since there's no
// space_shares row to return). A genuine stranger — no House relationship,
// no share — must still get a 404.
func TestHandleSpaceSharePinFallsBackToHouseMembership(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	fellowMember := mustCreateTestUser(t, app, "fellow@example.com")
	stranger := mustCreateTestUser(t, app, "stranger@example.com")

	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison Principale", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	if _, err := app.DB.AddHouseMember(ctx, house.ID, fellowMember.ID, "member"); err != nil {
		t.Fatalf("AddHouseMember: %v", err)
	}
	category, err := app.DB.CreateCustomCategory(ctx, owner.ID, "Vacances", "🏖️", "", 0)
	if err != nil {
		t.Fatalf("creating category: %v", err)
	}
	if _, err := app.DB.CreateList(ctx, "Courses vacances", "shopping", house.ID, &category.ID, ""); err != nil {
		t.Fatalf("creating list: %v", err)
	}

	// Sanity check: the fellow member really has no space_shares row —
	// otherwise this test would prove nothing about the fallback path.
	if _, err := app.DB.GetSpaceShare(ctx, category.ID, fellowMember.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("test setup bug: expected no pre-existing space_shares row, got %v", err)
	}

	categoryIDStr := strconv.FormatInt(category.ID, 10)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/custom-categories/"+categoryIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	req.SetPathValue("id", categoryIDStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, fellowMember))
	rec := httptest.NewRecorder()

	app.handleSpaceSharePin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 pinning a House-visible space via the fallback path, got %d: %s", rec.Code, rec.Body.String())
	}
	var status models.SpacePinStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if !status.IsPinnedToDashboard || status.AccessSource != "house_member" || status.CustomCategoryID != category.ID {
		t.Fatalf("expected a pinned house_member SpacePinStatus for the category, got %+v", status)
	}

	strangerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/custom-categories/"+categoryIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	strangerReq.SetPathValue("id", categoryIDStr)
	strangerReq = strangerReq.WithContext(context.WithValue(strangerReq.Context(), userContextKey, stranger))
	strangerRec := httptest.NewRecorder()

	app.handleSpaceSharePin(strangerRec, strangerReq)

	if strangerRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a stranger with no House relationship or share, got %d: %s", strangerRec.Code, strangerRec.Body.String())
	}

	// The category's own owner is also a House member of `house` (they
	// created both), which would make them pass spaceAccessibleViaHouse's
	// House-membership check if it didn't specifically exclude the
	// category's own owner — this asserts that exclusion actually holds
	// through the full handler, not just the db-layer unit test.
	ownerReq := httptest.NewRequest(http.MethodPatch, "/api/v1/custom-categories/"+categoryIDStr+"/share/pin", strings.NewReader(`{"pinned":true}`))
	ownerReq.SetPathValue("id", categoryIDStr)
	ownerReq = ownerReq.WithContext(context.WithValue(ownerReq.Context(), userContextKey, owner))
	ownerRec := httptest.NewRecorder()

	app.handleSpaceSharePin(ownerRec, ownerReq)

	if ownerRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for the category's own owner even though they're a House member where it's used, got %d: %s", ownerRec.Code, ownerRec.Body.String())
	}
}
