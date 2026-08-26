package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"trakka/internal/models"
)

// CreateHouseWithOwner atomically creates a house and adds ownerUserID as
// its owner, so a house is never left without a member. This is the first
// use of an explicit transaction in this package; a partial failure here
// (house row created but the membership insert failing) would otherwise
// strand a house nobody could ever access again via the RBAC-scoped
// listing/access-check methods.
func (d *DB) CreateHouseWithOwner(ctx context.Context, name string, ownerUserID int64) (*models.House, error) {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning house creation transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	res, err := tx.ExecContext(ctx, `INSERT INTO houses (name) VALUES (?)`, name)
	if err != nil {
		return nil, fmt.Errorf("inserting house: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading inserted house id: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO house_members (house_id, user_id, role) VALUES (?, ?, 'owner')`, id, ownerUserID,
	); err != nil {
		return nil, fmt.Errorf("adding house owner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing house creation: %w", err)
	}
	return d.GetHouse(ctx, id)
}

// GetHouse fetches a single house by id. Returns ErrNotFound if no such
// house exists.
func (d *DB) GetHouse(ctx context.Context, id int64) (*models.House, error) {
	h := &models.House{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM houses WHERE id = ?`, id,
	).Scan(&h.ID, &h.Name, &h.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying house %d: %w", id, err)
	}
	return h, nil
}

// UpdateHouse renames a house. Returns ErrNotFound if no such house exists.
func (d *DB) UpdateHouse(ctx context.Context, id int64, name string) (*models.House, error) {
	res, err := d.conn.ExecContext(ctx, `UPDATE houses SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return nil, fmt.Errorf("updating house %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("reading rows affected for house %d: %w", id, err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return d.GetHouse(ctx, id)
}

// DeleteHouse removes a house (its lists, and their items, cascade via
// ON DELETE CASCADE). Returns ErrNotFound if no such house exists.
func (d *DB) DeleteHouse(ctx context.Context, id int64) error {
	res, err := d.conn.ExecContext(ctx, `DELETE FROM houses WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting house %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for house %d: %w", id, err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
