package db

import (
	"context"
	"testing"
)

// TestListTotalAmountExcludesDoneItems is the regression guard for the
// dashboard card's price badge: total_amount must be the list's "reste à
// dépenser" — price * quantity summed only across still-unchecked items —
// never the list's full lifetime cost. A done/purchased item must never
// contribute, regardless of its price. See models.List.TotalAmount and
// static/js/app.js's urlBadges.
func TestListTotalAmountExcludesDoneItems(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}

	// A fresh list with no items at all has nothing to total.
	fresh, err := d.GetList(ctx, list.ID)
	if err != nil {
		t.Fatalf("GetList (fresh): %v", err)
	}
	if fresh.TotalAmount != nil {
		t.Fatalf("expected nil TotalAmount for a list with no items, got %v", *fresh.TotalAmount)
	}

	priced := 10.0
	pricedQty2 := 5.0
	unpriced := (*float64)(nil)

	// A priced, still-active (not done) item.
	activeItem, err := d.CreateItem(ctx, list.ID, "Actif", nil, 1, &priced, false, 0, nil, nil, nil, nil, false, nil, nil, false)
	if err != nil {
		t.Fatalf("creating priced item: %v", err)
	}
	// A priced item with quantity > 1: price * quantity, not just price.
	qty2Item, err := d.CreateItem(ctx, list.ID, "Quantité 2", nil, 2, &pricedQty2, false, 0, nil, nil, nil, nil, false, nil, nil, false)
	if err != nil {
		t.Fatalf("creating quantity-2 item: %v", err)
	}
	// An item with no price at all must not contribute (and must not zero
	// out the whole sum either).
	if _, err := d.CreateItem(ctx, list.ID, "Sans prix", nil, 1, unpriced, false, 0, nil, nil, nil, nil, false, nil, nil, false); err != nil {
		t.Fatalf("creating unpriced item: %v", err)
	}

	// Mark the quantity-2 item done — this is the whole point of the test:
	// a purchased/checked-off item must be excluded from total_amount, which
	// is the list's "reste à dépenser", not its full lifetime cost.
	if _, err := d.UpdateItem(ctx, qty2Item.ID, qty2Item.Title, qty2Item.URL, qty2Item.Quantity, qty2Item.Price, qty2Item.PriceAuto,
		qty2Item.ImageURL, true, qty2Item.Position, qty2Item.TargetMonth, qty2Item.DueDate, qty2Item.RecurrenceRule,
		qty2Item.RecurrenceEndDate, qty2Item.IsUrgent, qty2Item.RecurrenceLeadMinutes, qty2Item.TargetPrice, qty2Item.AlertOnPriceDrop); err != nil {
		t.Fatalf("marking item done: %v", err)
	}

	want := 10.0 // only the still-active item; the now-done 5*2 item is excluded

	got, err := d.GetList(ctx, list.ID)
	if err != nil {
		t.Fatalf("GetList: %v", err)
	}
	if got.TotalAmount == nil || *got.TotalAmount != want {
		t.Fatalf("GetList: expected TotalAmount %v, got %v", want, got.TotalAmount)
	}

	// ListListsForUser (the plain GET /api/v1/lists path) must agree.
	all, err := d.ListListsForUser(ctx, owner, "", house.ID)
	if err != nil {
		t.Fatalf("ListListsForUser: %v", err)
	}
	if len(all) != 1 || all[0].TotalAmount == nil || *all[0].TotalAmount != want {
		t.Fatalf("ListListsForUser: expected exactly one list with TotalAmount %v, got %+v", want, all)
	}

	// Mark the one remaining active/priced item done too: every priced item
	// in the list is now done, so the sum has nothing left to add over —
	// SUM() returns SQL NULL for zero matching rows, which must surface as
	// a nil TotalAmount (the "nothing to show" signal), not a stale value
	// or a spurious 0.
	if _, err := d.UpdateItem(ctx, activeItem.ID, activeItem.Title, activeItem.URL, activeItem.Quantity, activeItem.Price, activeItem.PriceAuto,
		activeItem.ImageURL, true, activeItem.Position, activeItem.TargetMonth, activeItem.DueDate, activeItem.RecurrenceRule,
		activeItem.RecurrenceEndDate, activeItem.IsUrgent, activeItem.RecurrenceLeadMinutes, activeItem.TargetPrice, activeItem.AlertOnPriceDrop); err != nil {
		t.Fatalf("marking last active item done: %v", err)
	}

	allDone, err := d.GetList(ctx, list.ID)
	if err != nil {
		t.Fatalf("GetList (all done): %v", err)
	}
	if allDone.TotalAmount != nil {
		t.Fatalf("expected nil TotalAmount once every priced item is done, got %v", *allDone.TotalAmount)
	}
}
