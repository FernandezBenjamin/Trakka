package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"trakka/internal/models"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(row rowScanner) (*models.Item, error) {
	it := &models.Item{}
	var done int
	var priceAuto int
	var isRecurring int
	var price sql.NullFloat64
	var imageURL sql.NullString
	var targetMonth sql.NullString
	var dueDate sql.NullString
	var recurrenceRule sql.NullString
	var recurrenceEndDate sql.NullString
	if err := row.Scan(&it.ID, &it.ListID, &it.Title, &it.URL, &it.Quantity, &done,
		&it.Position, &it.CreatedAt, &it.UpdatedAt, &price, &priceAuto, &imageURL, &targetMonth,
		&dueDate, &isRecurring, &recurrenceRule, &recurrenceEndDate); err != nil {
		return nil, err
	}
	it.Done = done != 0
	if price.Valid {
		it.Price = &price.Float64
	}
	it.PriceAuto = priceAuto != 0
	if imageURL.Valid && imageURL.String != "" {
		s := imageURL.String
		it.ImageURL = &s
	}
	if targetMonth.Valid && targetMonth.String != "" {
		s := targetMonth.String
		it.TargetMonth = &s
	}
	if dueDate.Valid && dueDate.String != "" {
		s := dueDate.String
		it.DueDate = &s
	}
	it.IsRecurring = isRecurring != 0
	if recurrenceRule.Valid && recurrenceRule.String != "" {
		s := recurrenceRule.String
		it.RecurrenceRule = &s
	}
	if recurrenceEndDate.Valid && recurrenceEndDate.String != "" {
		s := recurrenceEndDate.String
		it.RecurrenceEndDate = &s
	}
	return it, nil
}

// ListItemsByList returns all items of a list, ordered for display. Returns
// an empty slice (not an error) if the list has no items or does not exist;
// callers that need existence checked should call GetList first.
func (d *DB) ListItemsByList(ctx context.Context, listID int64) ([]*models.Item, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, list_id, title, url, quantity, done, position, created_at, updated_at, price, price_auto, image_url, target_month,
		 due_date, is_recurring, recurrence_rule, recurrence_end_date
		 FROM items WHERE list_id = ? ORDER BY position ASC, id ASC`, listID)
	if err != nil {
		return nil, fmt.Errorf("querying items for list %d: %w", listID, err)
	}
	defer rows.Close()

	items := []*models.Item{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning item row: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating item rows: %w", err)
	}
	return items, nil
}

// CreateItem inserts a new item under listID and returns the persisted row.
// priceAuto should always be false here — a freshly created item's price
// (even if nil) always comes directly from the request, never from the
// scraper (see scrapePrice in internal/handlers, which runs only after the
// item already exists). targetMonth is the planned purchase month
// (YYYY-MM, validated by internal/validate.Month) or nil if not scheduled.
// dueDate (YYYY-MM-DD) and recurrenceEndDate (YYYY-MM-DD) are validated by
// internal/validate.Date; recurrenceRule is validated by
// internal/validate.Recurrence. is_recurring is never a separate argument —
// it's stored as simply whether recurrenceRule is non-nil, so the two can
// never disagree.
func (d *DB) CreateItem(ctx context.Context, listID int64, title string, url *string, quantity int, price *float64, priceAuto bool, position int, targetMonth, dueDate, recurrenceRule, recurrenceEndDate *string) (*models.Item, error) {
	res, err := d.conn.ExecContext(ctx,
		`INSERT INTO items (list_id, title, url, quantity, price, price_auto, position, target_month, due_date, is_recurring, recurrence_rule, recurrence_end_date)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		listID, title, url, quantity, price, boolToInt(priceAuto), position, targetMonth, dueDate, boolToInt(recurrenceRule != nil), recurrenceRule, recurrenceEndDate)
	if err != nil {
		return nil, fmt.Errorf("inserting item: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading inserted item id: %w", err)
	}
	return d.GetItem(ctx, id)
}

// GetItem fetches a single item by id. Returns ErrNotFound if no such item
// exists.
func (d *DB) GetItem(ctx context.Context, id int64) (*models.Item, error) {
	row := d.conn.QueryRowContext(ctx,
		`SELECT id, list_id, title, url, quantity, done, position, created_at, updated_at, price, price_auto, image_url, target_month,
		 due_date, is_recurring, recurrence_rule, recurrence_end_date
		 FROM items WHERE id = ?`, id)
	item, err := scanItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying item %d: %w", id, err)
	}
	return item, nil
}

// UpdateItem replaces every mutable field of an item. priceAuto should be
// false for every caller except the PATCH handler preserving an existing
// scraped price it didn't touch (see internal/handlers/items.go); any
// caller that supplies price directly from the request is setting it
// manually, even when the value happens to be nil. imageURL should be the
// item's existing image_url when url isn't changing, or nil when it is —
// a scraped image is tied to the url it was found on, so a url change
// invalidates it the same way a manual price invalidates price_auto (see
// internal/handlers/items.go, which computes this before calling in).
// targetMonth is the planned purchase month (YYYY-MM) or nil to leave the
// item unscheduled. dueDate/recurrenceRule/recurrenceEndDate follow the
// same validation/is_recurring-derivation rules as CreateItem — callers
// (internal/handlers) are expected to have already run a recurring item's
// completion through applyRecurrenceCompletion before calling this, so
// done/dueDate here already reflect any auto-advance. Returns ErrNotFound
// if no such item exists.
func (d *DB) UpdateItem(ctx context.Context, id int64, title string, url *string, quantity int, price *float64, priceAuto bool, imageURL *string, done bool, position int, targetMonth, dueDate, recurrenceRule, recurrenceEndDate *string) (*models.Item, error) {
	res, err := d.conn.ExecContext(ctx,
		`UPDATE items SET title = ?, url = ?, quantity = ?, price = ?, price_auto = ?, image_url = ?, done = ?, position = ?, target_month = ?,
		 due_date = ?, is_recurring = ?, recurrence_rule = ?, recurrence_end_date = ?,
		 updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`,
		title, url, quantity, price, boolToInt(priceAuto), imageURL, boolToInt(done), position, targetMonth,
		dueDate, boolToInt(recurrenceRule != nil), recurrenceRule, recurrenceEndDate, id)
	if err != nil {
		return nil, fmt.Errorf("updating item %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("reading rows affected for item %d: %w", id, err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return d.GetItem(ctx, id)
}

// UpdateItemPriceIfMissing sets an item's price and marks it
// auto-detected, but only if the item still has no price and its url still
// matches the one that was scraped. internal/handlers' background price
// lookup (kicked off after an item is created or its url changes) uses
// this rather than UpdateItem so a slow scrape can never clobber a price
// the user typed in, or a price for a since-changed url, while it was in
// flight. Rows affected == 0 (item deleted, price set manually meanwhile,
// or url changed meanwhile) is an expected, silent no-op — there is
// nothing to report as an error.
func (d *DB) UpdateItemPriceIfMissing(ctx context.Context, id int64, url string, price float64) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE items SET price = ?, price_auto = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? AND url = ? AND price IS NULL`,
		price, id, url)
	if err != nil {
		return fmt.Errorf("saving scraped price for item %d: %w", id, err)
	}
	return nil
}

// UpdateItemImageIfMissing sets an item's image_url, but only if the item
// still has no image and its url still matches the one that was scraped —
// the same race guard as UpdateItemPriceIfMissing, and for the same reason:
// internal/handlers' background product lookup calls this rather than
// UpdateItem so a slow scrape can never overwrite an image for a
// since-changed url. Rows affected == 0 (item deleted, image already set,
// or url changed meanwhile) is an expected, silent no-op.
func (d *DB) UpdateItemImageIfMissing(ctx context.Context, id int64, url string, imageURL string) error {
	_, err := d.conn.ExecContext(ctx,
		`UPDATE items SET image_url = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? AND url = ? AND (image_url IS NULL OR image_url = '')`,
		imageURL, id, url)
	if err != nil {
		return fmt.Errorf("saving scraped image for item %d: %w", id, err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// DeleteItem removes an item. Returns ErrNotFound if no such item exists.
func (d *DB) DeleteItem(ctx context.Context, id int64) error {
	res, err := d.conn.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting item %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for item %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
