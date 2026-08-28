package handlers

import (
	"fmt"
	"time"

	"trakka/internal/models"
	"trakka/internal/validate"
)

// applyRecurrenceCompletion is what makes a recurring item behave
// differently from an ordinary one when it's checked off. It's called with
// wasDone set to the item's Done value *before* the request being handled
// was applied, and only ever does something on the false → true transition
// of a recurring item (item.RecurrenceRule set): instead of leaving the
// item marked done, it advances item.DueDate to the next occurrence (see
// nextDueDate) and flips item.Done back to false — the "décale simplement
// la date de la tâche actuelle" approach, chosen over cloning a new row per
// occurrence so a recurring chore never accumulates one item per
// completion. If item.RecurrenceEndDate is set and the computed next
// occurrence would fall after it, the recurrence has run its course: the
// item is left marked done, exactly like a plain non-recurring item, and
// this stops touching it on every future call (wasDone will be true from
// then on).
//
// Any other transition (already done, still not done, or a non-recurring
// item) is left untouched.
func applyRecurrenceCompletion(item *models.Item, wasDone bool) {
	if wasDone || !item.Done || item.RecurrenceRule == nil || *item.RecurrenceRule == "" {
		return
	}

	current := ""
	if item.DueDate != nil {
		current = *item.DueDate
	}
	next, err := nextDueDate(current, *item.RecurrenceRule)
	if err != nil {
		// Only reachable if a rule already persisted before validation was
		// tightened somehow slipped through — leave the item marked done
		// rather than advancing on a rule we can't actually interpret.
		return
	}

	if item.RecurrenceEndDate != nil && *item.RecurrenceEndDate != "" && next > *item.RecurrenceEndDate {
		return
	}

	item.DueDate = &next
	item.Done = false
}

// nextDueDate computes the next occurrence's due date for a recurring item,
// advancing from its current due date (or today, in UTC, if it doesn't have
// one yet) by rule — one of the fixed cadences or the custom
// "EVERY_X_DAYS:<n>" form, both already validated by
// internal/validate.Recurrence by the time this is called. Dates are plain
// YYYY-MM-DD, which stays lexicographically comparable — the same
// convention used elsewhere in this codebase for strftime timestamps.
func nextDueDate(currentDueDate, rule string) (string, error) {
	base := time.Now().UTC()
	if currentDueDate != "" {
		parsed, err := time.Parse("2006-01-02", currentDueDate)
		if err != nil {
			return "", fmt.Errorf("parsing current due date %q: %w", currentDueDate, err)
		}
		base = parsed
	}

	switch rule {
	case "DAILY":
		base = base.AddDate(0, 0, 1)
	case "WEEKLY":
		base = base.AddDate(0, 0, 7)
	case "MONTHLY":
		base = base.AddDate(0, 1, 0)
	case "YEARLY":
		base = base.AddDate(1, 0, 0)
	default:
		n, ok := validate.EveryXDaysInterval(rule)
		if !ok {
			return "", fmt.Errorf("unrecognized recurrence rule %q", rule)
		}
		base = base.AddDate(0, 0, n)
	}
	return base.Format("2006-01-02"), nil
}
