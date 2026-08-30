package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// decodeJSON decodes the request body into dst, rejecting unknown fields
// and bodies over 1 MiB. On failure it writes a 400 response itself and
// returns false, so callers can just `if !decodeJSON(...) { return }`.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	// Reject anything after the first JSON value. Without this, a body like
	// `{"done":true}{"done":false}` is accepted and the trailing document
	// silently ignored — an ambiguity worth refusing outright in a request
	// parser rather than resolving by accident.
	if dec.More() {
		writeError(w, http.StatusBadRequest, "invalid JSON body: unexpected data after the JSON document")
		return false
	}
	return true
}

// writeJSON writes data as a JSON response. encoding/json's default
// SetEscapeHTML(true) behavior (left untouched here) escapes '<', '>' and
// '&', so user-supplied text is safe even if this JSON is ever rendered
// into an HTML context.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// pathID parses the "{id}" path value as a positive integer. On failure it
// writes a 400 response itself and returns ok=false.
func pathID(w http.ResponseWriter, r *http.Request) (id int64, ok bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

// nullableString converts an empty string to nil, for storing optional
// text columns (e.g. item URL) as SQL NULL rather than "".
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// stringValue is nullableString's inverse: it reads an optional text
// column back out as "" when unset, for callers that need to compare a
// "was this actually changed" value (e.g. an item's URL before/after an
// edit) rather than round-trip it back into SQL.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
