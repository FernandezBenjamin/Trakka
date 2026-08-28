package handlers

import (
	"errors"
	"net/http"
	"net/url"

	"trakka/internal/auth"
	"trakka/internal/db"
	"trakka/internal/settings"
)

// loginPageData is what templates/login.html renders. Error is always
// mapped from a fixed, known ?error= code below — never raw query-string
// content — even though html/template auto-escapes regardless.
type loginPageData struct {
	OIDCEnabled      bool
	RegistrationOpen bool
	InstanceName     string
	Mode             string // "login" | "register"
	Error            string
}

var loginErrorMessages = map[string]string{ // #nosec G101 -- these are user-facing UI strings, not credentials
	"bad_request":         "Requête invalide, veuillez réessayer.",
	"invalid_credentials": "Email ou mot de passe incorrect.",
	"email_taken":         "Un compte existe déjà avec cet email.",
	"oidc_failed":         "La connexion via le fournisseur externe a échoué.",
	"weak_password":       "Le mot de passe doit contenir au moins 8 caractères.",
	"invalid_email":       "Adresse email invalide.",
	"registration_closed": "Les inscriptions sont actuellement fermées.",
}

func (app *Application) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if user, err := app.Auth.UserFromRequest(r); err == nil && user != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	current, err := settings.Resolve(r.Context(), app.DB, app.Config)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	mode := "login"
	if r.URL.Query().Get("mode") == "register" {
		mode = "register"
	}
	if mode == "register" && !current.RegistrationOpen {
		http.Redirect(w, r, "/auth/login?error=registration_closed", http.StatusFound)
		return
	}
	data := loginPageData{
		OIDCEnabled:      app.Auth.OIDC() != nil,
		RegistrationOpen: current.RegistrationOpen,
		InstanceName:     current.InstanceName,
		Mode:             mode,
		Error:            loginErrorMessages[r.URL.Query().Get("error")],
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := app.LoginTemplate.Execute(w, data); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *Application) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/auth/login?error=bad_request", http.StatusFound)
		return
	}

	user, err := app.Auth.Authenticate(r.Context(), r.FormValue("email"), r.FormValue("password"))
	if errors.Is(err, auth.ErrInvalidCredentials) {
		http.Redirect(w, r, "/auth/login?error=invalid_credentials", http.StatusFound)
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.finishLogin(w, r, user.ID)
}

func (app *Application) handleRegisterSubmit(w http.ResponseWriter, r *http.Request) {
	current, err := settings.Resolve(r.Context(), app.DB, app.Config)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !current.RegistrationOpen {
		http.Redirect(w, r, "/auth/login?mode=register&error=registration_closed", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/auth/login?mode=register&error=bad_request", http.StatusFound)
		return
	}

	if r.FormValue("password") != r.FormValue("password_confirm") {
		http.Redirect(w, r, "/auth/login?mode=register&error=bad_request", http.StatusFound)
		return
	}

	user, err := app.Auth.Register(r.Context(), r.FormValue("email"), r.FormValue("password"), r.FormValue("display_name"))
	switch {
	case errors.Is(err, db.ErrDuplicateEmail):
		http.Redirect(w, r, "/auth/login?mode=register&error=email_taken", http.StatusFound)
		return
	case err != nil:
		// Register() only otherwise fails on invalid input (bad email,
		// weak password) — a client-side problem, not a server one.
		http.Redirect(w, r, "/auth/login?mode=register&error=bad_request", http.StatusFound)
		return
	}

	if _, err := app.DB.CreateHouseWithOwner(r.Context(), auth.DefaultHouseName, user.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.finishLogin(w, r, user.ID)
}

func (app *Application) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		_ = app.Auth.RevokeSession(r.Context(), cookie.Value)
	}
	app.Auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/auth/login", http.StatusFound)
}

func (app *Application) finishLogin(w http.ResponseWriter, r *http.Request, userID int64) {
	rawToken, expiresAt, err := app.Auth.CreateSession(r.Context(), userID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.Auth.SetSessionCookie(w, rawToken, expiresAt)
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcFlowCookieName holds the one-time state/nonce/PKCE-verifier bundle
// between handleOIDCLogin and handleOIDCCallback. SameSite=Lax (not
// Strict): the browser must still send it on the cross-site top-level GET
// navigation the IdP redirects back with, which Strict would block.
const oidcFlowCookieName = "trakka_oidc_flow"

func (app *Application) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	oidcClient := app.Auth.OIDC()
	if oidcClient == nil {
		http.NotFound(w, r)
		return
	}

	state, err := auth.RandomToken(16)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	nonce, err := auth.RandomToken(16)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	verifier, challenge, err := auth.NewPKCE()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	flow := url.Values{"state": {state}, "nonce": {nonce}, "verifier": {verifier}}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set from CookieSecure (SESSION_COOKIE_SECURE), gosec only recognizes a literal `true`
		Name:     oidcFlowCookieName,
		Value:    flow.Encode(),
		Path:     "/auth/oidc",
		HttpOnly: true,
		Secure:   app.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, oidcClient.AuthorizationURL(state, nonce, challenge), http.StatusFound)
}

func (app *Application) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	oidcClient := app.Auth.OIDC()
	if oidcClient == nil {
		http.NotFound(w, r)
		return
	}

	flowCookie, cookieErr := r.Cookie(oidcFlowCookieName)
	// Clear the flow cookie unconditionally: it's single-use regardless of
	// whether this callback succeeds. Attributes mirror the cookie set in
	// handleOIDCLogin above so the browser matches it for deletion.
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set from CookieSecure (SESSION_COOKIE_SECURE), gosec only recognizes a literal `true`
		Name:     oidcFlowCookieName,
		Value:    "",
		Path:     "/auth/oidc",
		HttpOnly: true,
		Secure:   app.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	fail := func(code string) { http.Redirect(w, r, "/auth/login?error="+code, http.StatusFound) }

	if r.URL.Query().Get("error") != "" {
		fail("oidc_failed")
		return
	}
	if cookieErr != nil {
		fail("oidc_failed")
		return
	}
	flow, err := url.ParseQuery(flowCookie.Value)
	if err != nil {
		fail("oidc_failed")
		return
	}
	if r.URL.Query().Get("state") != flow.Get("state") {
		fail("oidc_failed")
		return
	}

	claims, err := oidcClient.Exchange(r.Context(), r.URL.Query().Get("code"), flow.Get("verifier"))
	if err != nil {
		app.Logger.Warn("oidc exchange failed", "error", err)
		fail("oidc_failed")
		return
	}
	if claims.Nonce != flow.Get("nonce") {
		fail("oidc_failed")
		return
	}

	user, err := app.Auth.LoginOrProvisionOIDCUser(r.Context(), claims)
	if errors.Is(err, auth.ErrOIDCEmailConflict) {
		fail("email_taken")
		return
	} else if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.finishLogin(w, r, user.ID)
}

func (app *Application) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userFromContext(r))
}
