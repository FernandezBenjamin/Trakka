package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"trakka/internal/models"
)

func scanCustomCategory(row rowScanner) (*models.CustomCategory, error) {
	c := &models.CustomCategory{}
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Icon, &c.Color, &c.Position, &c.CreatedAt); err != nil {
		return nil, err
	}
	return c, nil
}

// CreateCustomCategory inserts a new category owned by userID and returns
// the persisted row.
func (d *DB) CreateCustomCategory(ctx context.Context, userID int64, name, icon, color string, position int) (*models.CustomCategory, error) {
	res, err := d.conn.ExecContext(ctx,
		`INSERT INTO custom_categories (user_id, name, icon, color, position) VALUES (?, ?, ?, ?, ?)`,
		userID, name, icon, color, position)
	if err != nil {
		return nil, fmt.Errorf("inserting custom category: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading inserted custom category id: %w", err)
	}
	return d.GetCustomCategory(ctx, id)
}

// GetCustomCategory fetches a single category by id, regardless of owner —
// used to embed a category's details on a shared list, since any member of
// the list's house may see it even though only its owner may edit it (see
// GetCustomCategoryForUser below). Returns ErrNotFound if no such category
// exists.
func (d *DB) GetCustomCategory(ctx context.Context, id int64) (*models.CustomCategory, error) {
	row := d.conn.QueryRowContext(ctx,
		`SELECT id, user_id, name, icon, color, position, created_at FROM custom_categories WHERE id = ?`, id)
	c, err := scanCustomCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying custom category %d: %w", id, err)
	}
	return c, nil
}

// GetCustomCategoryForUser fetches a category by id, scoped to its owner —
// used by the CRUD handlers (and by list create/update, to validate an
// association) so a category can only ever be found by the user who owns
// it. Returns ErrNotFound both when the id doesn't exist at all and when it
// belongs to a different user, so a forbidden lookup can't be distinguished
// from a missing one.
func (d *DB) GetCustomCategoryForUser(ctx context.Context, id, userID int64) (*models.CustomCategory, error) {
	row := d.conn.QueryRowContext(ctx,
		`SELECT id, user_id, name, icon, color, position, created_at FROM custom_categories WHERE id = ? AND user_id = ?`,
		id, userID)
	c, err := scanCustomCategory(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying custom category %d for user %d: %w", id, userID, err)
	}
	return c, nil
}

// ListCustomCategoriesForUser returns every category userID owns, ordered
// for display (position, then id as a tiebreak).
func (d *DB) ListCustomCategoriesForUser(ctx context.Context, userID int64) ([]*models.CustomCategory, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT id, user_id, name, icon, color, position, created_at FROM custom_categories
		 WHERE user_id = ? ORDER BY position ASC, id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying custom categories for user %d: %w", userID, err)
	}
	defer rows.Close()

	categories := []*models.CustomCategory{}
	for rows.Next() {
		c, err := scanCustomCategory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning custom category row: %w", err)
		}
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating custom category rows: %w", err)
	}
	return categories, nil
}

// UpdateCustomCategoryForUser replaces a category's name/icon/color/position,
// scoped to its owner the same way GetCustomCategoryForUser is. Returns
// ErrNotFound if no such category exists for that user.
func (d *DB) UpdateCustomCategoryForUser(ctx context.Context, id, userID int64, name, icon, color string, position int) (*models.CustomCategory, error) {
	res, err := d.conn.ExecContext(ctx,
		`UPDATE custom_categories SET name = ?, icon = ?, color = ?, position = ? WHERE id = ? AND user_id = ?`,
		name, icon, color, position, id, userID)
	if err != nil {
		return nil, fmt.Errorf("updating custom category %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("reading rows affected for custom category %d: %w", id, err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return d.GetCustomCategory(ctx, id)
}

// DeleteCustomCategoryForUser removes a category, scoped to its owner the
// same way GetCustomCategoryForUser is. Any list referencing it has
// custom_category_id reset to NULL automatically (ON DELETE SET NULL in
// schema.sql) rather than being deleted itself. Returns ErrNotFound if no
// such category exists for that user.
func (d *DB) DeleteCustomCategoryForUser(ctx context.Context, id, userID int64) error {
	res, err := d.conn.ExecContext(ctx, `DELETE FROM custom_categories WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("deleting custom category %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for custom category %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
