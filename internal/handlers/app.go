// Package handlers implements the HTTP layer: routing, request
// validation, and translating db-layer results into JSON responses. It
// never touches database/sql directly — all persistence goes through
// internal/db.
package handlers

import (
	"html/template"
	"log/slog"
	"net/http"

	"trakka/internal/auth"
	"trakka/internal/config"
	"trakka/internal/db"
)

// Application holds the dependencies shared by every handler.
type Application struct {
	DB            *db.DB
	StaticDir     string
	Logger        *slog.Logger
	Auth          *auth.Service
	LoginTemplate *template.Template
	// Config is the env-var configuration loaded at startup. Handlers read
	// it for values that stay env-only even after the admin settings panel
	// was added — currently just BaseURL, needed to build the OIDC
	// redirect_uri when the admin (re)enables OIDC at runtime (see
	// internal/handlers/admin.go) — and pass it to internal/settings.Resolve
	// as the fallback under whatever's stored in system_settings.
	Config config.Config
}

// Routes builds the full HTTP handler: middleware chain + route table.
//
// /api/v1/... is gated behind RequireSession as a whole (wrapped once,
// below) rather than per-route — every JSON API endpoint requires a valid
// session. /auth/... and static files stay unauthenticated: /auth/... is
// how a session gets established in the first place, and gating static
// files would break the service worker's cache-first app-shell strategy
// (the client-side redirect in app.js's apiRequest() handles the UX layer
// on top of the API's 401s).
func (app *Application) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", app.handleHealthz)

	mux.HandleFunc("GET /auth/login", app.handleLoginPage)
	mux.HandleFunc("POST /auth/login", app.handleLoginSubmit)
	mux.HandleFunc("POST /auth/register", app.handleRegisterSubmit)
	mux.HandleFunc("POST /auth/logout", app.handleLogout)
	mux.HandleFunc("GET /auth/oidc/login", app.handleOIDCLogin)
	mux.HandleFunc("GET /auth/oidc/callback", app.handleOIDCCallback)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/v1/me", app.handleMe)

	apiMux.HandleFunc("GET /api/v1/houses", app.handleHousesIndex)
	apiMux.HandleFunc("POST /api/v1/houses", app.handleHousesCreate)
	apiMux.HandleFunc("GET /api/v1/houses/{id}", app.handleHousesShow)
	apiMux.HandleFunc("PUT /api/v1/houses/{id}", app.handleHousesUpdate)
	apiMux.HandleFunc("DELETE /api/v1/houses/{id}", app.handleHousesDelete)
	apiMux.HandleFunc("GET /api/v1/houses/{id}/members", app.handleHouseMembersIndex)
	apiMux.HandleFunc("POST /api/v1/houses/{id}/members", app.handleHouseMembersInvite)
	apiMux.HandleFunc("DELETE /api/v1/houses/{id}/members/{userId}", app.handleHouseMembersRemove)

	apiMux.HandleFunc("GET /api/v1/custom-categories", app.handleCustomCategoriesIndex)
	apiMux.HandleFunc("POST /api/v1/custom-categories", app.handleCustomCategoriesCreate)
	apiMux.HandleFunc("PUT /api/v1/custom-categories/{id}", app.handleCustomCategoriesUpdate)
	apiMux.HandleFunc("DELETE /api/v1/custom-categories/{id}", app.handleCustomCategoriesDelete)

	apiMux.HandleFunc("GET /api/v1/lists", app.handleListsIndex)
	apiMux.HandleFunc("POST /api/v1/lists", app.handleListsCreate)
	apiMux.HandleFunc("GET /api/v1/lists/{id}", app.handleListsShow)
	apiMux.HandleFunc("PUT /api/v1/lists/{id}", app.handleListsUpdate)
	apiMux.HandleFunc("DELETE /api/v1/lists/{id}", app.handleListsDelete)

	apiMux.HandleFunc("GET /api/v1/items", app.handleItemsIndex)
	apiMux.HandleFunc("POST /api/v1/items", app.handleItemsCreate)
	apiMux.HandleFunc("GET /api/v1/items/{id}", app.handleItemsShow)
	apiMux.HandleFunc("PUT /api/v1/items/{id}", app.handleItemsUpdate)
	apiMux.HandleFunc("PATCH /api/v1/items/{id}", app.handleItemsPatch)
	apiMux.HandleFunc("DELETE /api/v1/items/{id}", app.handleItemsDelete)
	apiMux.HandleFunc("POST /api/v1/items/{id}/price-check", app.handleItemsPriceCheck)

	apiMux.HandleFunc("GET /api/v1/price-alerts", app.handlePriceAlertsIndex)
	apiMux.HandleFunc("PATCH /api/v1/price-alerts/{id}", app.handlePriceAlertsUpdate)

	apiMux.HandleFunc("GET /api/v1/admin/settings", app.handleAdminSettingsShow)
	apiMux.HandleFunc("PATCH /api/v1/admin/settings", app.handleAdminSettingsUpdate)

	mux.Handle("/api/v1/", app.RequireSession(apiMux))
	mux.Handle("/", http.FileServer(http.Dir(app.StaticDir)))

	var handler http.Handler = mux
	handler = SecurityHeaders(handler)
	handler = Logging(app.Logger, handler)
	handler = Recover(app.Logger, handler)
	return handler
}

// serverError logs the underlying error (never exposed to the client) and
// writes a generic 500 response.
func (app *Application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.Logger.Error("unhandled error", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}
