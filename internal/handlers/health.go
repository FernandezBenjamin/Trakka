package handlers

import "net/http"

func (app *Application) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if err := app.DB.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
