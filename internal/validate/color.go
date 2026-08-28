package validate

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidColor is returned when a user-supplied category color isn't a
// hex color.
var ErrInvalidColor = errors.New("color must be a hex value like #RRGGBB or #RGB")

var hexColorPattern = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// Color trims and validates a user-supplied color string (a
// CustomCategory's accent color). An empty (or whitespace-only) input is
// valid and returns "" with no error, meaning "no color set". A non-empty
// input must be a 3- or 6-digit hex color prefixed with '#'.
func Color(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !hexColorPattern.MatchString(trimmed) {
		return "", ErrInvalidColor
	}
	return trimmed, nil
}
