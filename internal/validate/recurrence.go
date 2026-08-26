package validate

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ErrInvalidRecurrenceRule is returned when a user-supplied recurrence rule
// isn't one of the recognized forms.
var ErrInvalidRecurrenceRule = errors.New("recurrence_rule must be one of DAILY, WEEKLY, MONTHLY, YEARLY, or EVERY_X_DAYS:<n>")

var fixedRecurrenceRules = map[string]bool{
	"DAILY":   true,
	"WEEKLY":  true,
	"MONTHLY": true,
	"YEARLY":  true,
}

var everyXDaysPattern = regexp.MustCompile(`^EVERY_X_DAYS:([1-9][0-9]*)$`)

// Recurrence trims and validates a user-supplied recurrence rule string. An
// empty (or whitespace-only) input is valid and returns "" with no error,
// meaning "not recurring". A non-empty input must be one of the fixed
// cadences (DAILY/WEEKLY/MONTHLY/YEARLY, case-insensitive on input but
// normalized to upper case) or the custom "EVERY_X_DAYS:<n>" form (n a
// positive integer), e.g. "EVERY_X_DAYS:3" to repeat every 3 days.
func Recurrence(raw string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", nil
	}
	if fixedRecurrenceRules[trimmed] || everyXDaysPattern.MatchString(trimmed) {
		return trimmed, nil
	}
	return "", ErrInvalidRecurrenceRule
}

// EveryXDaysInterval reports the N in an already-validated "EVERY_X_DAYS:N"
// rule, or ok=false if rule isn't that form (including the fixed cadences,
// which callers should check for separately).
func EveryXDaysInterval(rule string) (n int, ok bool) {
	m := everyXDaysPattern.FindStringSubmatch(rule)
	if m == nil {
		return 0, false
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return v, true
}
