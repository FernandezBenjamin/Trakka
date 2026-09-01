package db

import (
	"context"
	"testing"
)

// TestListItemsForRecurringNotifyScan exercises the scan query's filter
// (recurring, not done, has a due date) and, separately, that
// MarkRecurringReminderSent excludes an item until its due date changes
// again — the "re-arms automatically" behavior
// ListItemsForRecurringNotifyScan's own doc comment describes.
func TestListItemsForRecurringNotifyScan(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	owner := mustCreateUser(t, ctx, d)
	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Tâches", "todo", house.ID, nil, "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	rule := "WEEKLY"
	due := "2026-01-10"
	lead := 120
	recurring, err := d.CreateItem(ctx, list.ID, "Sortir les poubelles", nil, 1, nil, false, 0, nil, &due, &rule, nil, false, &lead)
	if err != nil {
		t.Fatalf("creating recurring item: %v", err)
	}

	// A non-recurring item, a recurring-but-already-done item, and a
	// recurring item with no due date yet must never show up in the scan.
	if _, err := d.CreateItem(ctx, list.ID, "Tâche ponctuelle", nil, 1, nil, false, 0, nil, &due, nil, nil, false, nil); err != nil {
		t.Fatalf("creating one-off item: %v", err)
	}
	doneRule := "DAILY"
	doneItem, err := d.CreateItem(ctx, list.ID, "Déjà faite", nil, 1, nil, false, 0, nil, &due, &doneRule, nil, false, nil)
	if err != nil {
		t.Fatalf("creating done recurring item: %v", err)
	}
	if _, err := d.UpdateItem(ctx, doneItem.ID, doneItem.Title, doneItem.URL, doneItem.Quantity, doneItem.Price, doneItem.PriceAuto, doneItem.ImageURL,
		true, doneItem.Position, doneItem.TargetMonth, doneItem.DueDate, doneItem.RecurrenceRule, doneItem.RecurrenceEndDate, doneItem.IsUrgent, doneItem.RecurrenceLeadMinutes); err != nil {
		t.Fatalf("marking item done: %v", err)
	}
	noDueRule := "MONTHLY"
	if _, err := d.CreateItem(ctx, list.ID, "Sans échéance", nil, 1, nil, false, 0, nil, nil, &noDueRule, nil, false, nil); err != nil {
		t.Fatalf("creating no-due-date recurring item: %v", err)
	}

	candidates, err := d.ListItemsForRecurringNotifyScan(ctx)
	if err != nil {
		t.Fatalf("ListItemsForRecurringNotifyScan: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ItemID != recurring.ID {
		t.Fatalf("expected exactly the one eligible recurring item, got %+v", candidates)
	}
	if candidates[0].DueDate != due {
		t.Errorf("DueDate = %q, want %q", candidates[0].DueDate, due)
	}
	if candidates[0].LeadMinutes == nil || *candidates[0].LeadMinutes != lead {
		t.Errorf("LeadMinutes = %v, want %d", candidates[0].LeadMinutes, lead)
	}
	if candidates[0].ListID != list.ID {
		t.Errorf("ListID = %d, want %d", candidates[0].ListID, list.ID)
	}

	// Marking the reminder sent for the item's current due date excludes it
	// from the next scan...
	if err := d.MarkRecurringReminderSent(ctx, recurring.ID, due); err != nil {
		t.Fatalf("MarkRecurringReminderSent: %v", err)
	}
	afterMark, err := d.ListItemsForRecurringNotifyScan(ctx)
	if err != nil {
		t.Fatalf("ListItemsForRecurringNotifyScan (after mark): %v", err)
	}
	if len(afterMark) != 0 {
		t.Fatalf("expected the reminded-for item to be excluded, got %+v", afterMark)
	}

	// ...but the moment its due_date changes to something new, it re-arms
	// automatically with no separate "clear the flag" call needed.
	newDue := "2026-01-17"
	refetched, err := d.GetItem(ctx, recurring.ID)
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if _, err := d.UpdateItem(ctx, refetched.ID, refetched.Title, refetched.URL, refetched.Quantity, refetched.Price, refetched.PriceAuto, refetched.ImageURL,
		refetched.Done, refetched.Position, refetched.TargetMonth, &newDue, refetched.RecurrenceRule, refetched.RecurrenceEndDate, refetched.IsUrgent, refetched.RecurrenceLeadMinutes); err != nil {
		t.Fatalf("advancing due date: %v", err)
	}
	rearmed, err := d.ListItemsForRecurringNotifyScan(ctx)
	if err != nil {
		t.Fatalf("ListItemsForRecurringNotifyScan (after due date change): %v", err)
	}
	if len(rearmed) != 1 || rearmed[0].ItemID != recurring.ID || rearmed[0].DueDate != newDue {
		t.Fatalf("expected the item to re-arm with its new due date, got %+v", rearmed)
	}
}
