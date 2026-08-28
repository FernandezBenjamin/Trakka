package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"trakka/internal/models"
)

// listSelect is shared by every read below. The LEFT JOIN embeds the
// attached custom category's own columns (all NULL when
// lists.custom_category_id is NULL, or the category was deleted out from
// under a stale reference — impossible here since deleting a category sets
// custom_category_id back to NULL via ON DELETE SET NULL, but scanListRow
// handles it defensively regardless) so a single query can return a list
// with its category embedded, matching the API's documented shape for
// GET /api/v1/lists and GET /api/v1/lists/{id}.
const listSelect = `
	SELECT lists.id, lists.house_id, lists.name, lists.type, lists.created_at, lists.updated_at, lists.custom_category_id,
	       cc.id, cc.user_id, cc.name, cc.icon, cc.color, cc.position, cc.created_at
	FROM lists LEFT JOIN custom_categories cc ON cc.id = lists.custom_category_id`

func scanListRow(row rowScanner) (*models.List, error) {
	l := &models.List{}
	var customCategoryID sql.NullInt64
	var catID sql.NullInt64
	var catUserID sql.NullInt64
	var catName, catIcon, catColor, catCreatedAt sql.NullString
	var catPosition sql.NullInt64
	if err := row.Scan(&l.ID, &l.HouseID, &l.Name, &l.Type, &l.CreatedAt, &l.UpdatedAt, &customCategoryID,
		&catID, &catUserID, &catName, &catIcon, &catColor, &catPosition, &catCreatedAt); err != nil {
		return nil, err
	}
	if customCategoryID.Valid {
		id := customCategoryID.Int64
		l.CustomCategoryID = &id
	}
	if catID.Valid {
		l.CustomCategory = &models.CustomCategory{
			ID:        catID.Int64,
			UserID:    catUserID.Int64,
			Name:      catName.String,
			Icon:      catIcon.String,
			Color:     catColor.String,
			Position:  int(catPosition.Int64),
			CreatedAt: catCreatedAt.String,
		}
	}
	return l, nil
}

// ListListsForUser returns lists belonging to houses userID is a member
// of, newest first. If typeFilter is non-empty, it restricts the result to
// lists of that type. If houseID is > 0, it further restricts the result
// to that one house (the caller should authorize access to it separately;
// this method silently returns nothing for a house the user isn't in).
func (d *DB) ListListsForUser(ctx context.Context, userID int64, typeFilter string, houseID int64) ([]*models.List, error) {
	query := listSelect + ` JOIN house_members hm ON hm.house_id = lists.house_id AND hm.user_id = ?`
	conditions := []string{}
	args := []any{userID}
	if typeFilter != "" {
		conditions = append(conditions, `lists.type = ?`)
		args = append(args, typeFilter)
	}
	if houseID > 0 {
		conditions = append(conditions, `lists.house_id = ?`)
		args = append(args, houseID)
	}
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY lists.created_at DESC`

	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying lists: %w", err)
	}
	defer rows.Close()

	lists := []*models.List{}
	for rows.Next() {
		l, err := scanListRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning list row: %w", err)
		}
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating list rows: %w", err)
	}
	return lists, nil
}

// CreateList inserts a new list under houseID and returns the persisted
// row. customCategoryID optionally attaches it to one of the caller's
// CustomCategory "spaces" (nil leaves it unattached) — the caller
// (internal/handlers) is expected to have already validated that it
// references a category the requesting user owns.
func (d *DB) CreateList(ctx context.Context, name, listType string, houseID int64, customCategoryID *int64) (*models.List, error) {
	res, err := d.conn.ExecContext(ctx,
		`INSERT INTO lists (name, type, house_id, custom_category_id) VALUES (?, ?, ?, ?)`,
		name, listType, houseID, customCategoryID)
	if err != nil {
		return nil, fmt.Errorf("inserting list: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading inserted list id: %w", err)
	}
	return d.GetList(ctx, id)
}

// GetList fetches a single list by id (without its items), with its
// attached custom category embedded if any. Returns ErrNotFound if no such
// list exists.
func (d *DB) GetList(ctx context.Context, id int64) (*models.List, error) {
	row := d.conn.QueryRowContext(ctx, listSelect+` WHERE lists.id = ?`, id)
	l, err := scanListRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying list %d: %w", id, err)
	}
	return l, nil
}

// UpdateList replaces a list's name, type, and custom category association
// (nil dissociates it). Returns ErrNotFound if no such list exists.
func (d *DB) UpdateList(ctx context.Context, id int64, name, listType string, customCategoryID *int64) (*models.List, error) {
	res, err := d.conn.ExecContext(ctx,
		`UPDATE lists SET name = ?, type = ?, custom_category_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`,
		name, listType, customCategoryID, id)
	if err != nil {
		return nil, fmt.Errorf("updating list %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("reading rows affected for list %d: %w", id, err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return d.GetList(ctx, id)
}

// DeleteList removes a list (its items cascade via ON DELETE CASCADE).
// Returns ErrNotFound if no such list exists.
func (d *DB) DeleteList(ctx context.Context, id int64) error {
	res, err := d.conn.ExecContext(ctx, `DELETE FROM lists WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting list %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for list %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
