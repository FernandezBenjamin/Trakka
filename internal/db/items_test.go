package db

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "trakka.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// TestUpdateItemPriceIfMissing exercises the three-way guard
// (UpdateItemPriceIfMissing's WHERE clause) that internal/handlers'
// background price scrape relies on to never clobber a manual price or a
// price meant for a different (since-changed) url.
func TestUpdateItemPriceIfMissing(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", mustCreateUser(t, ctx, d))
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID)
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}

	url := "https://example.com/product"

	t.Run("sets price when missing and url matches", func(t *testing.T) {
		item, err := d.CreateItem(ctx, list.ID, "Article", &url, 1, nil, false, 0, nil)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		if err := d.UpdateItemPriceIfMissing(ctx, item.ID, url, 9.99); err != nil {
			t.Fatalf("UpdateItemPriceIfMissing: %v", err)
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.Price == nil || *got.Price != 9.99 {
			t.Fatalf("expected price 9.99, got %v", got.Price)
		}
		if !got.PriceAuto {
			t.Fatal("expected price_auto to be true")
		}
	})

	t.Run("does not override an existing price", func(t *testing.T) {
		manual := 5.0
		item, err := d.CreateItem(ctx, list.ID, "Article manuel", &url, 1, &manual, false, 0, nil)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		if err := d.UpdateItemPriceIfMissing(ctx, item.ID, url, 9.99); err != nil {
			t.Fatalf("UpdateItemPriceIfMissing: %v", err)
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.Price == nil || *got.Price != 5.0 {
			t.Fatalf("expected manual price 5.0 to survive, got %v", got.Price)
		}
		if got.PriceAuto {
			t.Fatal("expected price_auto to remain false for a manual price")
		}
	})

	t.Run("does not set price when url no longer matches", func(t *testing.T) {
		item, err := d.CreateItem(ctx, list.ID, "Article changé", &url, 1, nil, false, 0, nil)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		other := "https://example.com/other-product"
		if err := d.UpdateItemPriceIfMissing(ctx, item.ID, other, 9.99); err != nil {
			t.Fatalf("UpdateItemPriceIfMissing: %v", err)
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.Price != nil {
			t.Fatalf("expected price to remain nil, got %v", *got.Price)
		}
	})
}

// TestUpdateItemImageIfMissing exercises the same three-way guard as
// TestUpdateItemPriceIfMissing above, but for UpdateItemImageIfMissing —
// internal/handlers' background product lookup relies on it to never
// clobber an image already set, or one meant for a different (since
// changed) url.
func TestUpdateItemImageIfMissing(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", mustCreateUser(t, ctx, d))
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID)
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}

	url := "https://example.com/product"

	t.Run("sets image when missing and url matches", func(t *testing.T) {
		item, err := d.CreateItem(ctx, list.ID, "Article", &url, 1, nil, false, 0, nil)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		if err := d.UpdateItemImageIfMissing(ctx, item.ID, url, "https://cdn.example.com/a.jpg"); err != nil {
			t.Fatalf("UpdateItemImageIfMissing: %v", err)
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.ImageURL == nil || *got.ImageURL != "https://cdn.example.com/a.jpg" {
			t.Fatalf("expected the scraped image, got %v", got.ImageURL)
		}
	})

	t.Run("does not override an existing image", func(t *testing.T) {
		item, err := d.CreateItem(ctx, list.ID, "Article", &url, 1, nil, false, 0, nil)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		if err := d.UpdateItemImageIfMissing(ctx, item.ID, url, "https://cdn.example.com/first.jpg"); err != nil {
			t.Fatalf("UpdateItemImageIfMissing: %v", err)
		}
		if err := d.UpdateItemImageIfMissing(ctx, item.ID, url, "https://cdn.example.com/second.jpg"); err != nil {
			t.Fatalf("UpdateItemImageIfMissing: %v", err)
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.ImageURL == nil || *got.ImageURL != "https://cdn.example.com/first.jpg" {
			t.Fatalf("expected the first image to survive, got %v", got.ImageURL)
		}
	})

	t.Run("does not set image when url no longer matches", func(t *testing.T) {
		item, err := d.CreateItem(ctx, list.ID, "Article changé", &url, 1, nil, false, 0, nil)
		if err != nil {
			t.Fatalf("creating item: %v", err)
		}
		other := "https://example.com/other-product"
		if err := d.UpdateItemImageIfMissing(ctx, item.ID, other, "https://cdn.example.com/a.jpg"); err != nil {
			t.Fatalf("UpdateItemImageIfMissing: %v", err)
		}
		got, err := d.GetItem(ctx, item.ID)
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}
		if got.ImageURL != nil {
			t.Fatalf("expected image to remain nil, got %v", *got.ImageURL)
		}
	})
}

func mustCreateUser(t *testing.T, ctx context.Context, d *DB) int64 {
	t.Helper()
	hash := "x"
	user, err := d.CreateUser(ctx, "owner@example.com", &hash, nil, nil, "Owner")
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	return user.ID
}
