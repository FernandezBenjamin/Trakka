package db

import (
	"context"
	"testing"
)

// targetPriceScanFixture creates a House/owner/shopping List, matching the
// minimal setup every test below needs before creating items in it.
func targetPriceScanFixture(t *testing.T, ctx context.Context, d *DB) (houseID, listID int64) {
	t.Helper()
	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", mustCreateUser(t, ctx, d))
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}
	return house.ID, list.ID
}

// TestListItemsForTargetPriceScan exercises the eligibility filter the
// periodic target-price worker relies on: only a not-done item with a url,
// alert_on_price_drop enabled, and a target_price set qualifies — an item
// missing any one of those is excluded, and unlike ListItemsForPriceScan an
// item's own current price is not required.
func TestListItemsForTargetPriceScan(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	_, listID := targetPriceScanFixture(t, ctx, d)

	url := "https://example.com/product"
	target := 20.0

	eligible, err := d.CreateItem(ctx, listID, "Éligible", &url, 1, nil, false, 0, nil, nil, nil, nil, false, nil, &target, true)
	if err != nil {
		t.Fatalf("creating eligible item: %v", err)
	}
	eligibleWithPrice := 25.0
	eligiblePriced, err := d.CreateItem(ctx, listID, "Éligible avec prix", &url, 1, &eligibleWithPrice, false, 0, nil, nil, nil, nil, false, nil, &target, true)
	if err != nil {
		t.Fatalf("creating eligible-with-price item: %v", err)
	}

	if _, err := d.CreateItem(ctx, listID, "Sans URL", nil, 1, nil, false, 0, nil, nil, nil, nil, false, nil, &target, true); err != nil {
		t.Fatalf("creating url-less item: %v", err)
	}
	if _, err := d.CreateItem(ctx, listID, "Alerte désactivée", &url, 1, nil, false, 0, nil, nil, nil, nil, false, nil, &target, false); err != nil {
		t.Fatalf("creating opted-out item: %v", err)
	}
	if _, err := d.CreateItem(ctx, listID, "Sans seuil", &url, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, true); err != nil {
		t.Fatalf("creating threshold-less item: %v", err)
	}

	done, err := d.CreateItem(ctx, listID, "Terminé", &url, 1, nil, false, 0, nil, nil, nil, nil, false, nil, &target, true)
	if err != nil {
		t.Fatalf("creating done item: %v", err)
	}
	if _, err := d.UpdateItem(ctx, done.ID, done.Title, done.URL, done.Quantity, done.Price, done.PriceAuto, done.ImageURL,
		true, done.Position, done.TargetMonth, done.DueDate, done.RecurrenceRule, done.RecurrenceEndDate, false, done.RecurrenceLeadMinutes, done.TargetPrice, true); err != nil {
		t.Fatalf("marking item done: %v", err)
	}

	items, err := d.ListItemsForTargetPriceScan(ctx)
	if err != nil {
		t.Fatalf("ListItemsForTargetPriceScan: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected exactly the two eligible items, got %+v", items)
	}
	gotIDs := map[int64]bool{items[0].ID: true, items[1].ID: true}
	if !gotIDs[eligible.ID] || !gotIDs[eligiblePriced.ID] {
		t.Fatalf("expected items %d and %d, got %+v", eligible.ID, eligiblePriced.ID, items)
	}
}

// TestUpdateItemPriceFromScan exercises the compare-and-swap guard: it only
// applies when both the url and the price the caller last read still match
// what's in the database, so a concurrent edit (or a stale read) can never
// be clobbered by a slow re-scrape landing late.
func TestUpdateItemPriceFromScan(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	_, listID := targetPriceScanFixture(t, ctx, d)
	url := "https://example.com/product"

	t.Run("applies when oldPrice is nil and matches", func(t *testing.T) {
		item, err := d.CreateItem(ctx, listID, "Sans prix", &url, 1, nil, false, 0, nil, nil, nil, nil, false, nil, nil, false)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		applied, err := d.UpdateItemPriceFromScan(ctx, item.ID, url, nil, 42.0)
		if err != nil {
			t.Fatalf("UpdateItemPriceFromScan: %v", err)
		}
		if !applied {
			t.Fatal("expected the update to apply")
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.Price == nil || *got.Price != 42.0 {
			t.Fatalf("expected price 42.0, got %v", got.Price)
		}
		if !got.PriceAuto {
			t.Fatal("expected price_auto to be true after a scan-applied price")
		}
	})

	t.Run("applies when oldPrice matches a non-nil current price", func(t *testing.T) {
		price := 30.0
		item, err := d.CreateItem(ctx, listID, "Avec prix", &url, 1, &price, false, 0, nil, nil, nil, nil, false, nil, nil, false)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		applied, err := d.UpdateItemPriceFromScan(ctx, item.ID, url, &price, 25.0)
		if err != nil {
			t.Fatalf("UpdateItemPriceFromScan: %v", err)
		}
		if !applied {
			t.Fatal("expected the update to apply")
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.Price == nil || *got.Price != 25.0 {
			t.Fatalf("expected price 25.0, got %v", got.Price)
		}
	})

	t.Run("does not apply when the current price has changed since oldPrice was read", func(t *testing.T) {
		price := 30.0
		item, err := d.CreateItem(ctx, listID, "Avec prix modifié", &url, 1, &price, false, 0, nil, nil, nil, nil, false, nil, nil, false)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		staleOldPrice := 30.0
		// Simulate someone else changing the price after this scan's stale
		// read but before its scrape finished.
		if _, err := d.UpdateItemPriceFromScan(ctx, item.ID, url, &staleOldPrice, 18.0); err != nil {
			t.Fatalf("UpdateItemPriceFromScan (setup change): %v", err)
		}

		applied, err := d.UpdateItemPriceFromScan(ctx, item.ID, url, &staleOldPrice, 10.0)
		if err != nil {
			t.Fatalf("UpdateItemPriceFromScan: %v", err)
		}
		if applied {
			t.Fatal("expected the update to lose the compare-and-swap race")
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.Price == nil || *got.Price != 18.0 {
			t.Fatalf("expected price to remain 18.0 (the intervening change), got %v", got.Price)
		}
	})

	t.Run("does not apply when the url has changed since it was read", func(t *testing.T) {
		price := 30.0
		item, err := d.CreateItem(ctx, listID, "Avec url", &url, 1, &price, false, 0, nil, nil, nil, nil, false, nil, nil, false)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		applied, err := d.UpdateItemPriceFromScan(ctx, item.ID, "https://example.com/different-product", &price, 5.0)
		if err != nil {
			t.Fatalf("UpdateItemPriceFromScan: %v", err)
		}
		if applied {
			t.Fatal("expected the update to be rejected for a stale url")
		}
	})

	t.Run("does not apply to a deleted item", func(t *testing.T) {
		price := 30.0
		item, err := d.CreateItem(ctx, listID, "À supprimer", &url, 1, &price, false, 0, nil, nil, nil, nil, false, nil, nil, false)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		if err := d.DeleteItem(ctx, item.ID); err != nil {
			t.Fatalf("deleting item: %v", err)
		}
		applied, err := d.UpdateItemPriceFromScan(ctx, item.ID, url, &price, 5.0)
		if err != nil {
			t.Fatalf("UpdateItemPriceFromScan: %v", err)
		}
		if applied {
			t.Fatal("expected the update to be a no-op for a deleted item")
		}
	})
}
