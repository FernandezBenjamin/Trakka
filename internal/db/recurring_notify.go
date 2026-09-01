package db

import (
	"context"
	"database/sql"
	"fmt"
)

// RecurringDueCandidate is one recurring, not-yet-done item whose due date
// hasn't triggered a lead-time reminder push yet for its *current*
// occurrence — see internal/handlers.RunRecurringDueScan. Deliberately not
// models.Item: due_reminder_sent_for is pure internal bookkeeping, never
// part of the API-facing Item shape, so this scan has no reason to pay for
// scanning/allocating every other Item field it doesn't need.
type RecurringDueCandidate struct {
	ItemID  int64
	ListID  int64
	Title   string
	DueDate string
	// LeadMinutes is the item's own recurrence_lead_minutes override, or nil
	// to use the instance-wide NOTIF_RECURRING_TASK_LEAD_TIME default — see
	// models.Item.RecurrenceLeadMinutes.
	LeadMinutes *int
}

// ListItemsForRecurringNotifyScan returns every recurring, not-done item
// with a due date that hasn't already had a reminder sent for that exact
// due date (see MarkRecurringReminderSent). Excluding an
// already-reminded-for due date here, rather than in Go after fetching
// everything, keeps the periodic scan cheap regardless of how many
// recurring items an instance accumulates, and re-arms itself automatically
// the moment due_date changes for any reason (advancing to the next
// occurrence on completion, or a manual edit) — due_reminder_sent_for
// simply stops matching due_date, with no explicit "clear the flag" step
// needed anywhere else in the codebase.
func (d *DB) ListItemsForRecurringNotifyScan(ctx context.Context) ([]*RecurringDueCandidate, error) {
	rows, err := d.conn.QueryContext(ctx, `
		SELECT id, list_id, title, due_date, recurrence_lead_minutes
		FROM items
		WHERE recurrence_rule IS NOT NULL AND done = 0 AND due_date IS NOT NULL
		  AND (due_reminder_sent_for IS NULL OR due_reminder_sent_for != due_date)`)
	if err != nil {
		return nil, fmt.Errorf("querying items for recurring notify scan: %w", err)
	}
	defer rows.Close()

	candidates := []*RecurringDueCandidate{}
	for rows.Next() {
		c := &RecurringDueCandidate{}
		var leadMinutes sql.NullInt64
		if err := rows.Scan(&c.ItemID, &c.ListID, &c.Title, &c.DueDate, &leadMinutes); err != nil {
			return nil, fmt.Errorf("scanning recurring notify candidate row: %w", err)
		}
		if leadMinutes.Valid {
			n := int(leadMinutes.Int64)
			c.LeadMinutes = &n
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recurring notify candidate rows: %w", err)
	}
	return candidates, nil
}

// MarkRecurringReminderSent records that a lead-time reminder has been sent
// for itemID's due date exactly as given — internal/handlers.RunRecurringDueScan
// calls this right after a successful send so the same occurrence isn't
// re-notified on the next scan tick. Storing the due_date value itself
// (rather than a plain boolean/timestamp flag) is what makes the "re-arm on
// due_date change" behavior described on ListItemsForRecurringNotifyScan
// above automatic: once the item's due_date advances past dueDate for any
// reason, this stored value simply stops matching it. A no-op (not an
// error) if the item no longer exists — the scan already has its own
// snapshot of the row and there is nothing left to update.
func (d *DB) MarkRecurringReminderSent(ctx context.Context, itemID int64, dueDate string) error {
	if _, err := d.conn.ExecContext(ctx,
		`UPDATE items SET due_reminder_sent_for = ? WHERE id = ?`, dueDate, itemID); err != nil {
		return fmt.Errorf("marking recurring reminder sent for item %d: %w", itemID, err)
	}
	return nil
}
