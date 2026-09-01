package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestListUpdateCannotRetagIntoAnotherUsersSpace is a regression guard for a
// privilege-escalation path found during the security audit.
//
// A list's custom_category_id decides which Space grants access to it
// (db.AccessLevelForList). handleListsUpdate accepted that field from anyone
// with *write* access to the list, which a list_shares/space_shares grant is
// enough to confer — so a share recipient could move a list they were merely
// lent into a Space they own, then share that Space with anyone, handing out
// access to a list they were never permitted to share (handleListShareCreate
// deliberately requires actual House membership for exactly that reason).
// Changing the category now requires House membership too.
func TestListUpdateCannotRetagIntoAnotherUsersSpace(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	recipient := mustCreateTestUser(t, app, "recipient@example.com")
	third := mustCreateTestUser(t, app, "third@example.com")

	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Liste privée", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	if _, err := app.DB.CreateOrUpdateListShare(ctx, list.ID, recipient.ID, "write"); err != nil {
		t.Fatalf("sharing list: %v", err)
	}
	category, err := app.DB.CreateCustomCategory(ctx, recipient.ID, "Mon espace", "", "", 0)
	if err != nil {
		t.Fatalf("creating category: %v", err)
	}

	idStr := strconv.FormatInt(list.ID, 10)
	body := `{"name":"Liste privée","type":"shopping","icon":"","custom_category_id":` +
		strconv.FormatInt(category.ID, 10) + `}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lists/"+idStr, strings.NewReader(body))
	req.SetPathValue("id", idStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, recipient))
	rec := httptest.NewRecorder()

	app.handleListsUpdate(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when a write-share recipient re-tags a list into their own space, got %d: %s",
			rec.Code, rec.Body.String())
	}

	// And the escalation the 403 exists to prevent must not have happened:
	// a third party the recipient shares their Space with still has no
	// access to the owner's list.
	if _, err := app.DB.CreateOrUpdateSpaceShare(ctx, category.ID, third.ID, "write"); err != nil {
		t.Fatalf("sharing space: %v", err)
	}
	updated, err := app.DB.GetList(ctx, list.ID)
	if err != nil {
		t.Fatalf("re-reading list: %v", err)
	}
	level, err := app.DB.AccessLevelForList(ctx, third.ID, updated)
	if err != nil {
		t.Fatalf("AccessLevelForList: %v", err)
	}
	if level != "" {
		t.Fatalf("third party gained %q access to a list nobody shared with them", level)
	}
}

// TestListUpdateAllowsHouseMemberToRetag is the other half of the rule above:
// the restriction must not have cost an ordinary House member the ability to
// organize their own house's lists into a Space.
func TestListUpdateAllowsHouseMemberToRetag(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	category, err := app.DB.CreateCustomCategory(ctx, owner.ID, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("creating category: %v", err)
	}

	idStr := strconv.FormatInt(list.ID, 10)
	body := `{"name":"Courses","type":"shopping","icon":"","custom_category_id":` +
		strconv.FormatInt(category.ID, 10) + `}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lists/"+idStr, strings.NewReader(body))
	req.SetPathValue("id", idStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, owner))
	rec := httptest.NewRecorder()

	app.handleListsUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a House member re-tagging their own list, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListUpdateByShareRecipientKeepsWorkingForOtherFields confirms the fix
// is scoped to the category: a write-share holder can still rename a list.
func TestListUpdateByShareRecipientKeepsWorkingForOtherFields(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	recipient := mustCreateTestUser(t, app, "recipient@example.com")

	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Ancien nom", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	if _, err := app.DB.CreateOrUpdateListShare(ctx, list.ID, recipient.ID, "write"); err != nil {
		t.Fatalf("sharing list: %v", err)
	}

	idStr := strconv.FormatInt(list.ID, 10)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/lists/"+idStr,
		strings.NewReader(`{"name":"Nouveau nom","type":"shopping","icon":"🛒"}`))
	req.SetPathValue("id", idStr)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, recipient))
	rec := httptest.NewRecorder()

	app.handleListsUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 renaming a write-shared list, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestRequireSameOriginWrite covers the cross-origin write guard added as a
// second CSRF layer under the session cookie's SameSite=Lax attribute.
func TestRequireSameOriginWrite(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := requireSameOriginWrite("trakka.example.com", next)

	cases := []struct {
		name    string
		method  string
		host    string
		origin  string
		fetch   string
		want    int
		comment string
	}{
		{"same-origin POST", http.MethodPost, "trakka.example.com", "https://trakka.example.com", "", http.StatusNoContent, ""},
		{"same-origin POST with port", http.MethodPost, "localhost:8080", "http://localhost:8080", "", http.StatusNoContent, ""},
		{"cross-origin POST", http.MethodPost, "trakka.example.com", "https://evil.example", "", http.StatusForbidden, ""},
		{"cross-origin DELETE", http.MethodDelete, "trakka.example.com", "https://evil.example", "", http.StatusForbidden, ""},
		// A same-origin navigational POST (an HTML <form>, not fetch()) under
		// Referrer-Policy: no-referrer legitimately carries Origin: null per
		// the Fetch spec — see the dedicated comment in requireSameOriginWrite.
		// This is exactly what /auth/login's own <form> sends in every real
		// browser, and must be let through when Sec-Fetch-Site vouches for it.
		{"opaque null origin, same-origin fetch metadata", http.MethodPost, "trakka.example.com", "null", "same-origin", http.StatusNoContent, "a real browser's no-referrer-policy navigation must not be rejected"},
		// An opaque origin (a sandboxed iframe forging Origin: null the same
		// way) is never same-site with anything, so Sec-Fetch-Site still
		// correctly reads cross-site here — the case this check originally
		// existed to catch is still caught.
		{"opaque null origin, cross-site fetch metadata", http.MethodPost, "trakka.example.com", "null", "cross-site", http.StatusForbidden, "an opaque-origin CSRF attempt must still be rejected"},
		// No Sec-Fetch-Site at all alongside Origin: null only happens on a
		// browser old enough to lack Fetch Metadata support entirely — the
		// same "no usable signal -> allow" trade-off already accepted for
		// "no headers at all (curl)" below. /auth/login and /auth/register
		// are additionally covered by their own double-submit csrf_token
		// (checkCSRFToken), and every /api/v1/... write still requires the
		// SameSite=Lax session cookie, which such a request would not carry
		// cross-site either way — so this narrow gap grants nothing usable.
		{"opaque null origin, no fetch metadata at all", http.MethodPost, "trakka.example.com", "null", "", http.StatusNoContent, "very old browsers with no Sec-Fetch-Site are the same accepted gap as a non-browser client"},
		{"proxied host, BASE_URL origin", http.MethodPost, "trakka-internal", "https://trakka.example.com", "", http.StatusNoContent, ""},
		{"no origin, cross-site fetch metadata", http.MethodPost, "trakka.example.com", "", "cross-site", http.StatusForbidden, ""},
		{"no origin, same-origin fetch metadata", http.MethodPost, "trakka.example.com", "", "same-origin", http.StatusNoContent, ""},
		{"no headers at all (curl)", http.MethodPost, "trakka.example.com", "", "", http.StatusNoContent, "non-browser clients are not a CSRF vector"},
		{"cross-origin GET is a read", http.MethodGet, "trakka.example.com", "https://evil.example", "", http.StatusNoContent, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1/lists", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetch != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetch)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d (%s)", rec.Code, tc.want, tc.comment)
			}
		})
	}
}

// TestAuthRateLimiter checks the fixed-window counter behind login/register
// throttling: attempts inside the budget pass, the one after it does not, and
// a successful login's reset restores the budget.
func TestAuthRateLimiter(t *testing.T) {
	rl := newRateLimiter(authRateWindow)

	for i := 0; i < 3; i++ {
		if !rl.allow("user@example.com", 3) {
			t.Fatalf("attempt %d should have been allowed", i+1)
		}
	}
	if rl.allow("user@example.com", 3) {
		t.Fatal("the fourth attempt should have been rejected")
	}
	if !rl.allow("other@example.com", 3) {
		t.Fatal("a different key must have its own independent budget")
	}

	rl.reset("user@example.com")
	if !rl.allow("user@example.com", 3) {
		t.Fatal("reset (a successful login) should restore the budget")
	}
}

// TestDecodeJSONRejectsTrailingData guards the request decoder against a body
// carrying more than one JSON document.
func TestDecodeJSONRejectsTrailingData(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/houses", strings.NewReader(`{"name":"a"}{"name":"b"}`))
	rec := httptest.NewRecorder()

	if decodeJSON(rec, req, &dst) {
		t.Fatal("expected a body with trailing data to be rejected")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

// TestInviteDoesNotDiscloseAccountExistence is the regression guard for the
// account-enumeration oracle (docs/AUDIT.md, L-06).
//
// The invite and share endpoints used to resolve the submitted address to a
// user and answer 404 "no account exists for this email yet" when there was
// none, which let any authenticated user test arbitrary email addresses for
// an account on the instance. The response must now be byte-for-byte
// indistinguishable between a registered and an unregistered address, apart
// from the address being echoed back.
func TestInviteDoesNotDiscloseAccountExistence(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	mustCreateTestUser(t, app, "registered@example.com")

	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := app.DB.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	category, err := app.DB.CreateCustomCategory(ctx, owner.ID, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("creating category: %v", err)
	}

	houseID := strconv.FormatInt(house.ID, 10)
	listID := strconv.FormatInt(list.ID, 10)
	categoryID := strconv.FormatInt(category.ID, 10)

	// probe posts one invite/share and returns the status and body, with the
	// probed address blanked out so two probes are directly comparable.
	probe := func(t *testing.T, handler http.HandlerFunc, path, id, body, email string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.SetPathValue("id", id)
		req = req.WithContext(context.WithValue(req.Context(), userContextKey, owner))
		rec := httptest.NewRecorder()
		handler(rec, req)
		// Blank the echoed address and the row id/timestamp, which legitimately
		// differ between two distinct invitations.
		out := strings.ReplaceAll(rec.Body.String(), email, "<EMAIL>")
		out = regexp.MustCompile(`"(id|created_at)":("[^"]*"|\d+)`).ReplaceAllString(out, `"$1":<X>`)
		return rec.Code, out
	}

	cases := []struct {
		name    string
		handler http.HandlerFunc
		path    string
		id      string
		body    func(email string) string
	}{
		{
			"house invite", app.handleHouseMembersInvite,
			"/api/v1/houses/" + houseID + "/members", houseID,
			func(e string) string { return `{"email":"` + e + `"}` },
		},
		{
			"list share", app.handleListShareCreate,
			"/api/v1/lists/" + listID + "/share", listID,
			func(e string) string { return `{"email":"` + e + `","permission":"read"}` },
		},
		{
			"space share", app.handleSpaceShareCreate,
			"/api/v1/custom-categories/" + categoryID + "/share", categoryID,
			func(e string) string { return `{"email":"` + e + `","permission":"read"}` },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			knownCode, knownBody := probe(t, tc.handler, tc.path, tc.id,
				tc.body("registered@example.com"), "registered@example.com")
			unknownCode, unknownBody := probe(t, tc.handler, tc.path, tc.id,
				tc.body("nobody@example.com"), "nobody@example.com")

			if knownCode != http.StatusCreated || unknownCode != http.StatusCreated {
				t.Fatalf("expected 201 for both, got known=%d unknown=%d\n  known:   %s\n  unknown: %s",
					knownCode, unknownCode, knownBody, unknownBody)
			}
			if knownBody != unknownBody {
				t.Fatalf("response discloses whether the address is registered:\n  known:   %s\n  unknown: %s",
					knownBody, unknownBody)
			}
		})
	}
}

// TestInviteGrantsNothingUntilRecipientSignsIn confirms the other half of the
// property: because the reply is uniform, the invitation must genuinely not
// take effect yet — otherwise the roster would leak what the response no
// longer does.
func TestInviteGrantsNothingUntilRecipientSignsIn(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	owner := mustCreateTestUser(t, app, "owner@example.com")
	invitee := mustCreateTestUser(t, app, "invitee@example.com")

	house, err := app.DB.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	houseID := strconv.FormatInt(house.ID, 10)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/houses/"+houseID+"/members",
		strings.NewReader(`{"email":"invitee@example.com"}`))
	req.SetPathValue("id", houseID)
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, owner))
	rec := httptest.NewRecorder()
	app.handleHouseMembersInvite(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite failed: %d %s", rec.Code, rec.Body.String())
	}

	canAccess, err := app.DB.UserCanAccessHouse(ctx, invitee.ID, house.ID)
	if err != nil {
		t.Fatalf("UserCanAccessHouse: %v", err)
	}
	if canAccess {
		t.Fatal("an invitation must not grant membership before the invitee signs in")
	}

	// Signing in (handleMe) is what applies it.
	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq = meReq.WithContext(context.WithValue(meReq.Context(), userContextKey, invitee))
	app.handleMe(httptest.NewRecorder(), meReq)

	canAccess, err = app.DB.UserCanAccessHouse(ctx, invitee.ID, house.ID)
	if err != nil {
		t.Fatalf("UserCanAccessHouse: %v", err)
	}
	if !canAccess {
		t.Fatal("the invitation should have been applied when the invitee signed in")
	}
}
