// Package validate holds small, reusable input-validation helpers shared by
// the handlers layer.
package validate

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidURL is returned when a user-supplied URL is not an absolute
// http:// or https:// URL.
var ErrInvalidURL = errors.New("url must be an absolute http:// or https:// URL")

// URL trims and validates a user-supplied URL string. An empty (or
// whitespace-only) input is valid and returns "" with no error, meaning
// "no URL provided". A non-empty input must parse as an absolute URL with
// scheme http or https and a non-empty host; this blocks schemes such as
// "javascript:" or "data:" from ever being persisted or reflected back by
// the API.
func URL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ErrInvalidURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidURL
	}
	if parsed.Host == "" {
		return "", ErrInvalidURL
	}

	return trimmed, nil
}
