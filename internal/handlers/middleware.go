package handlers

import (
	"log/slog"
	"net/http"
	"time"
)

// SecurityHeaders sets standard hardening headers on every response.
// static/index.html loads the Tailwind Play CDN script, which is the one
// deliberate, narrowly-scoped exception to an otherwise unsafe-inline-free
// policy: script-src trusts exactly that one host (no 'unsafe-inline', no
// 'unsafe-eval' — Trakka's own scripts stay external-file-only), and
// style-src needs 'unsafe-inline' because Play CDN generates utility CSS
// into a <style> tag at runtime rather than serving an external file.
// img-src's "http: https:" is the other deliberate widening: an item's
// product thumbnail (internal/scraper's og:image/JSON-LD/twitter:image
// lookup, static/js/list_view.js's buildItemThumbnail) can point at any
// storefront's CDN, the same way an item's own url already links out
// anywhere — there is no fixed allowlist of image hosts to name here, so
// img-src has to trust the scheme rather than a host list. The actual
// scraped URL is still constrained the same way item.url is: scraper.
// resolveImageURL only ever accepts an absolute http(s) URL (see
// internal/scraper/scraper.go), so this can never be used to load a
// "javascript:"/"data:" URI dressed up as an image source.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' https://cdn.tailwindcss.com; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: http: https:; "+
				"connect-src 'self'; "+
				"worker-src 'self'; "+
				"manifest-src 'self'; "+
				"object-src 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'; "+
				"frame-ancestors 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// Logging records method, path, status, and duration for every request.
func Logging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// Recover is a last-resort safety net against an unexpected panic (e.g. a
// bug surfacing deep in a dependency). It is not a substitute for the
// explicit error handling used throughout the handlers and db packages —
// every predictable failure is already handled with a returned error
// before it would ever reach here.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "path", r.URL.Path, "recovered", rec)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(status int) {
	sw.status = status
	sw.ResponseWriter.WriteHeader(status)
}
