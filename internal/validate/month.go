package validate

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidMonth is returned when a user-supplied target month is not in
// YYYY-MM format.
var ErrInvalidMonth = errors.New("target_month must be in YYYY-MM format")

var monthPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// Month trims and validates a user-supplied "target month" string (an
// item's planned purchase month, e.g. "2026-11"). An empty (or
// whitespace-only) input is valid and returns "" with no error, meaning "no
// target month set". A non-empty input must match YYYY-MM with a month
// component between 01 and 12.
func Month(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if !monthPattern.MatchString(trimmed) {
		return "", ErrInvalidMonth
	}
	return trimmed, nil
}
