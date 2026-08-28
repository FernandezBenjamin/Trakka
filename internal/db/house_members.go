package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"trakka/internal/models"
)

// AddHouseMember adds userID to houseID with the given role. Returns
// ErrAlreadyMember if userID is already a member of houseID.
func (d *DB) AddHouseMember(ctx context.Context, houseID, userID int64, role string) (*models.HouseMember, error) {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO house_members (house_id, user_id, role) VALUES (?, ?, ?)`, houseID, userID, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return nil, ErrAlreadyMember
		}
		return nil, fmt.Errorf("inserting house member: %w", err)
	}
	return d.GetHouseMember(ctx, houseID, userID)
}

// ListHouseMembers returns every member of houseID, owners first, joined
// with the user's email and display name for display purposes.
func (d *DB) ListHouseMembers(ctx context.Context, houseID int64) ([]*models.HouseMember, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT hm.house_id, hm.user_id, hm.role, hm.created_at, u.email, u.display_name
		 FROM house_members hm JOIN users u ON u.id = hm.user_id
		 WHERE hm.house_id = ?
		 ORDER BY (hm.role = 'owner') DESC, hm.created_at ASC`, houseID)
	if err != nil {
		return nil, fmt.Errorf("querying house members for house %d: %w", houseID, err)
	}
	defer rows.Close()

	members := []*models.HouseMember{}
	for rows.Next() {
		m := &models.HouseMember{}
		if err := rows.Scan(&m.HouseID, &m.UserID, &m.Role, &m.CreatedAt, &m.Email, &m.DisplayName); err != nil {
			return nil, fmt.Errorf("scanning house member row: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating house member rows: %w", err)
	}
	return members, nil
}

// GetHouseMember fetches a single membership row. Returns ErrNotFound if
// userID is not a member of houseID.
func (d *DB) GetHouseMember(ctx context.Context, houseID, userID int64) (*models.HouseMember, error) {
	m := &models.HouseMember{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT house_id, user_id, role, created_at FROM house_members WHERE house_id = ? AND user_id = ?`,
		houseID, userID,
	).Scan(&m.HouseID, &m.UserID, &m.Role, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying house member: %w", err)
	}
	return m, nil
}

// RemoveHouseMember removes userID from houseID. Returns ErrNotFound if
// userID is not a member of houseID.
func (d *DB) RemoveHouseMember(ctx context.Context, houseID, userID int64) error {
	res, err := d.conn.ExecContext(ctx,
		`DELETE FROM house_members WHERE house_id = ? AND user_id = ?`, houseID, userID)
	if err != nil {
		return fmt.Errorf("deleting house member: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for house member delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListHousesForUser returns every house userID is a member of, oldest
// first. This replaces the unscoped ListHouses for RBAC-aware callers.
func (d *DB) ListHousesForUser(ctx context.Context, userID int64) ([]*models.House, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT h.id, h.name, h.created_at FROM houses h
		 JOIN house_members hm ON hm.house_id = h.id
		 WHERE hm.user_id = ? ORDER BY h.id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying houses for user %d: %w", userID, err)
	}
	defer rows.Close()

	houses := []*models.House{}
	for rows.Next() {
		h := &models.House{}
		if err := rows.Scan(&h.ID, &h.Name, &h.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning house row: %w", err)
		}
		houses = append(houses, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating house rows: %w", err)
	}
	return houses, nil
}

// UserCanAccessHouse reports whether userID is a member (any role) of
// houseID.
func (d *DB) UserCanAccessHouse(ctx context.Context, userID, houseID int64) (bool, error) {
	var exists bool
	err := d.conn.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM house_members WHERE house_id = ? AND user_id = ?)`, houseID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking house access: %w", err)
	}
	return exists, nil
}

// UserRoleInHouse returns userID's role in houseID. Returns ErrNotFound if
// userID is not a member of houseID.
func (d *DB) UserRoleInHouse(ctx context.Context, userID, houseID int64) (string, error) {
	var role string
	err := d.conn.QueryRowContext(ctx,
		`SELECT role FROM house_members WHERE house_id = ? AND user_id = ?`, houseID, userID,
	).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("querying house role: %w", err)
	}
	return role, nil
}
