package validate

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidDate is returned when a user-supplied calendar date is not in
// YYYY-MM-DD format.
var ErrInvalidDate = errors.New("date must be in YYYY-MM-DD format")

// Date trims and validates a user-supplied calendar date string (e.g. an
// item's due_date or recurrence_end_date). An empty (or whitespace-only)
// input is valid and returns "" with no error, meaning "no date set". A
// non-empty input must be a real calendar date in YYYY-MM-DD form.
func Date(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", trimmed); err != nil {
		return "", ErrInvalidDate
	}
	return trimmed, nil
}
