package handlers

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

	"trakka/internal/auth"
)

// CSRF defenses, in two independent layers.
//
// Layer 1 — SameSite=Lax on the session cookie (internal/auth.Service.
// SetSessionCookie) — predates this file and is still the primary control
// for /api/v1/...: Lax withholds the cookie from every cross-site request
// except a top-level GET navigation, and no GET route this app exposes
// mutates anything.
//
// Layer 2, added by this file, covers the two gaps Lax alone leaves:
//
//   - A cross-site POST to /auth/login or /auth/register carries no session
//     cookie (there is none yet), so SameSite protects nothing there. An
//     attacker could therefore submit *their own* credentials from a page the
//     victim visits and silently sign the victim into the attacker's account
//     ("login CSRF"), after which everything the victim types goes into a
//     list the attacker can read. csrfToken/checkCSRFToken below implement a
//     signed-nothing, compare-everything double-submit: the login page is
//     rendered with a hidden field whose value must match an HttpOnly cookie
//     the same response set, which a cross-site attacker can neither read nor
//     predict.
//
//   - A cross-site POST to /auth/logout does carry no session cookie either,
//     but its response still clears one — so it could log a victim out as a
//     nuisance. requireSameOriginWrite below rejects it.
//
// requireSameOriginWrite additionally re-checks every state-changing API
// request against the Origin/Sec-Fetch-Site headers, so the JSON API no
// longer rests on SameSite alone.

// csrfCookieName holds the double-submit token for the unauthenticated
// /auth/... forms. Scoped to /auth so it is never sent with API or static
// requests that have no use for it.
const csrfCookieName = "trakka_csrf" // #nosec G101 -- cookie name, not a credential

// csrfTokenField is the hidden form field templates/login.html renders.
const csrfTokenField = "csrf_token"

// ensureCSRFToken returns the token to render into a form, reusing the one
// already in the request's cookie when there is one so that several tabs (or
// a page restored from the back/forward cache) all stay valid, and minting +
// setting a fresh one otherwise.
func (app *Application) ensureCSRFToken(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie(csrfCookieName); err == nil && len(cookie.Value) >= 22 {
		return cookie.Value, nil
	}
	token, err := auth.RandomToken(32)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set from CookieSecure (SESSION_COOKIE_SECURE), gosec only recognizes a literal `true`
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   app.Auth.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((12 * 60 * 60)), // 12h: long enough for a login page left open, short enough to rotate
	})
	return token, nil
}

// checkCSRFToken reports whether the submitted form token matches the
// cookie. Both halves must be present; a missing cookie (e.g. the form was
// posted without ever loading the page) fails closed. The comparison is
// constant-time out of habit rather than necessity — the token is compared
// against a value the same client supplied, not a stored secret — but it
// costs nothing and keeps the pattern correct if this is ever reused.
func checkCSRFToken(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	submitted := r.PostFormValue(csrfTokenField)
	if submitted == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(submitted)) == 1
}

// requireSameOriginWrite rejects any state-changing request that a browser
// tells us came from another site.
//
// Two signals are used, both set by the browser itself and neither
// forgeable from page JavaScript:
//
//   - Origin, which every modern browser attaches to POST/PUT/PATCH/DELETE
//     (same-origin ones included). Its host must match the host this request
//     was addressed to. Only the host is compared, never the scheme: behind a
//     TLS-terminating reverse proxy the browser's Origin says "https" while
//     r.Host carries only the hostname, and requiring a scheme match would
//     break every such deployment for no security gain.
//   - Sec-Fetch-Site, checked when Origin is absent.
//
// A request with neither header is allowed through: that is a non-browser
// client (curl, a script, the container healthcheck), which is not a CSRF
// vector — CSRF is by definition an attack that borrows a browser's ambient
// credentials.
func requireSameOriginWrite(allowedHost string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}

		if origin := r.Header.Get("Origin"); origin != "" {
			if !originMatchesHost(origin, r.Host, allowedHost) {
				writeError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
		} else if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
			writeError(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// originMatchesHost reports whether an Origin header names the same host as
// the request was addressed to, or the host of an explicitly configured
// BASE_URL (which covers a proxy that rewrites Host).
func originMatchesHost(origin, requestHost, allowedHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false // "null" and other opaque origins are not same-origin
	}
	originHost := strings.ToLower(parsed.Hostname())
	if originHost == "" {
		return false
	}
	if originHost == strings.ToLower(hostOnly(requestHost)) {
		return true
	}
	return allowedHost != "" && originHost == allowedHost
}

// hostOnly strips any :port suffix from a Host header value.
func hostOnly(host string) string {
	if i := strings.LastIndex(host, ":"); i != -1 && !strings.Contains(host[i:], "]") {
		return host[:i]
	}
	return host
}
