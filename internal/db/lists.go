package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"trakka/internal/models"
)

// ListListsForUser returns lists belonging to houses userID is a member
// of, newest first. If typeFilter is non-empty, it restricts the result to
// lists of that type. If houseID is > 0, it further restricts the result
// to that one house (the caller should authorize access to it separately;
// this method silently returns nothing for a house the user isn't in).
func (d *DB) ListListsForUser(ctx context.Context, userID int64, typeFilter string, houseID int64) ([]*models.List, error) {
	query := `SELECT lists.id, lists.house_id, lists.name, lists.type, lists.created_at, lists.updated_at
		FROM lists JOIN house_members hm ON hm.house_id = lists.house_id AND hm.user_id = ?`
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
		l := &models.List{}
		if err := rows.Scan(&l.ID, &l.HouseID, &l.Name, &l.Type, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning list row: %w", err)
		}
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating list rows: %w", err)
	}
	return lists, nil
}

// CreateList inserts a new list under houseID and returns the persisted row.
func (d *DB) CreateList(ctx context.Context, name, listType string, houseID int64) (*models.List, error) {
	res, err := d.conn.ExecContext(ctx,
		`INSERT INTO lists (name, type, house_id) VALUES (?, ?, ?)`, name, listType, houseID)
	if err != nil {
		return nil, fmt.Errorf("inserting list: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading inserted list id: %w", err)
	}
	return d.GetList(ctx, id)
}

// GetList fetches a single list by id (without its items). Returns
// ErrNotFound if no such list exists.
func (d *DB) GetList(ctx context.Context, id int64) (*models.List, error) {
	l := &models.List{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, house_id, name, type, created_at, updated_at FROM lists WHERE id = ?`, id,
	).Scan(&l.ID, &l.HouseID, &l.Name, &l.Type, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying list %d: %w", id, err)
	}
	return l, nil
}

// UpdateList replaces a list's name and type. Returns ErrNotFound if no
// such list exists.
func (d *DB) UpdateList(ctx context.Context, id int64, name, listType string) (*models.List, error) {
	res, err := d.conn.ExecContext(ctx,
		`UPDATE lists SET name = ?, type = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`,
		name, listType, id)
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
