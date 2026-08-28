package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"trakka/internal/models"
)

func scanSpaceShare(row rowScanner) (*models.SpaceShare, error) {
	s := &models.SpaceShare{}
	if err := row.Scan(&s.ID, &s.CustomCategoryID, &s.SharedWithUserID, &s.Permission, &s.CreatedAt); err != nil {
		return nil, err
	}
	return s, nil
}

func scanListShare(row rowScanner) (*models.ListShare, error) {
	s := &models.ListShare{}
	if err := row.Scan(&s.ID, &s.ListID, &s.SharedWithUserID, &s.Permission, &s.CreatedAt); err != nil {
		return nil, err
	}
	return s, nil
}

// CreateOrUpdateSpaceShare grants sharedWithUserID `permission` access to
// categoryID, upserting on the table's (custom_category_id,
// shared_with_user_id) UNIQUE index so re-sharing with a different
// permission updates the existing row rather than erroring. categoryID is
// expected to already be known to exist and be owned by the caller —
// validated by the handler before calling this (see
// handleSpaceShareCreate), the same division of responsibility as
// validateCustomCategoryOwnership in internal/handlers/lists.go.
func (d *DB) CreateOrUpdateSpaceShare(ctx context.Context, categoryID, sharedWithUserID int64, permission string) (*models.SpaceShare, error) {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO space_shares (custom_category_id, shared_with_user_id, permission) VALUES (?, ?, ?)
		ON CONFLICT (custom_category_id, shared_with_user_id) DO UPDATE SET permission = excluded.permission
	`, categoryID, sharedWithUserID, permission)
	if err != nil {
		return nil, fmt.Errorf("upserting space share: %w", err)
	}
	return d.GetSpaceShare(ctx, categoryID, sharedWithUserID)
}

// GetSpaceShare fetches a single share row. Returns ErrNotFound if
// sharedWithUserID has no share for categoryID.
func (d *DB) GetSpaceShare(ctx context.Context, categoryID, sharedWithUserID int64) (*models.SpaceShare, error) {
	row := d.conn.QueryRowContext(ctx,
		`SELECT id, custom_category_id, shared_with_user_id, permission, created_at
		 FROM space_shares WHERE custom_category_id = ? AND shared_with_user_id = ?`,
		categoryID, sharedWithUserID)
	s, err := scanSpaceShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying space share: %w", err)
	}
	return s, nil
}

// ListSpaceShares returns every share on categoryID, joined with the
// recipient's email/display name for display purposes — mirrors
// ListHouseMembers.
func (d *DB) ListSpaceShares(ctx context.Context, categoryID int64) ([]*models.SpaceShare, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT ss.id, ss.custom_category_id, ss.shared_with_user_id, ss.permission, ss.created_at, u.email, u.display_name
		 FROM space_shares ss JOIN users u ON u.id = ss.shared_with_user_id
		 WHERE ss.custom_category_id = ? ORDER BY ss.created_at ASC`, categoryID)
	if err != nil {
		return nil, fmt.Errorf("querying space shares for category %d: %w", categoryID, err)
	}
	defer rows.Close()

	shares := []*models.SpaceShare{}
	for rows.Next() {
		s := &models.SpaceShare{}
		if err := rows.Scan(&s.ID, &s.CustomCategoryID, &s.SharedWithUserID, &s.Permission, &s.CreatedAt, &s.Email, &s.DisplayName); err != nil {
			return nil, fmt.Errorf("scanning space share row: %w", err)
		}
		shares = append(shares, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating space share rows: %w", err)
	}
	return shares, nil
}

// RevokeSpaceShare removes a share. Returns ErrNotFound if sharedWithUserID
// has no share for categoryID.
func (d *DB) RevokeSpaceShare(ctx context.Context, categoryID, sharedWithUserID int64) error {
	res, err := d.conn.ExecContext(ctx,
		`DELETE FROM space_shares WHERE custom_category_id = ? AND shared_with_user_id = ?`, categoryID, sharedWithUserID)
	if err != nil {
		return fmt.Errorf("deleting space share: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for space share delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateOrUpdateListShare grants sharedWithUserID `permission` access to
// listID — the List equivalent of CreateOrUpdateSpaceShare, upserting the
// same way.
func (d *DB) CreateOrUpdateListShare(ctx context.Context, listID, sharedWithUserID int64, permission string) (*models.ListShare, error) {
	_, err := d.conn.ExecContext(ctx, `
		INSERT INTO list_shares (list_id, shared_with_user_id, permission) VALUES (?, ?, ?)
		ON CONFLICT (list_id, shared_with_user_id) DO UPDATE SET permission = excluded.permission
	`, listID, sharedWithUserID, permission)
	if err != nil {
		return nil, fmt.Errorf("upserting list share: %w", err)
	}
	return d.GetListShare(ctx, listID, sharedWithUserID)
}

// GetListShare fetches a single share row. Returns ErrNotFound if
// sharedWithUserID has no share for listID.
func (d *DB) GetListShare(ctx context.Context, listID, sharedWithUserID int64) (*models.ListShare, error) {
	row := d.conn.QueryRowContext(ctx,
		`SELECT id, list_id, shared_with_user_id, permission, created_at
		 FROM list_shares WHERE list_id = ? AND shared_with_user_id = ?`,
		listID, sharedWithUserID)
	s, err := scanListShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying list share: %w", err)
	}
	return s, nil
}

// ListListShares returns every share on listID, joined with the recipient's
// email/display name — mirrors ListSpaceShares.
func (d *DB) ListListShares(ctx context.Context, listID int64) ([]*models.ListShare, error) {
	rows, err := d.conn.QueryContext(ctx,
		`SELECT ls.id, ls.list_id, ls.shared_with_user_id, ls.permission, ls.created_at, u.email, u.display_name
		 FROM list_shares ls JOIN users u ON u.id = ls.shared_with_user_id
		 WHERE ls.list_id = ? ORDER BY ls.created_at ASC`, listID)
	if err != nil {
		return nil, fmt.Errorf("querying list shares for list %d: %w", listID, err)
	}
	defer rows.Close()

	shares := []*models.ListShare{}
	for rows.Next() {
		s := &models.ListShare{}
		if err := rows.Scan(&s.ID, &s.ListID, &s.SharedWithUserID, &s.Permission, &s.CreatedAt, &s.Email, &s.DisplayName); err != nil {
			return nil, fmt.Errorf("scanning list share row: %w", err)
		}
		shares = append(shares, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating list share rows: %w", err)
	}
	return shares, nil
}

// RevokeListShare removes a share. Returns ErrNotFound if sharedWithUserID
// has no share for listID.
func (d *DB) RevokeListShare(ctx context.Context, listID, sharedWithUserID int64) error {
	res, err := d.conn.ExecContext(ctx,
		`DELETE FROM list_shares WHERE list_id = ? AND shared_with_user_id = ?`, listID, sharedWithUserID)
	if err != nil {
		return fmt.Errorf("deleting list share: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for list share delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AccessLevelForList reports the highest access level userID has to list:
// "write" if they're a member (any role) of its House — House membership
// has always implied full read/write access to every one of its lists, so
// this preserves that exactly — "write" or "read" if they hold a
// list_shares grant of that level directly, or the same via a space_shares
// grant on the list's parent Space (list.CustomCategoryID), whichever of
// the two is higher if both apply, or "" if none of the three apply. This
// is the single place that implements CLAUDE.md's granular-sharing rule:
// "un utilisateur doit pouvoir lire/modifier une liste s'il possède l'accès
// à la Maison parent, OU à l'Espace parent, OU directement à la Liste
// partagée."
func (d *DB) AccessLevelForList(ctx context.Context, userID int64, list *models.List) (string, error) {
	isMember, err := d.UserCanAccessHouse(ctx, userID, list.HouseID)
	if err != nil {
		return "", err
	}
	if isMember {
		return "write", nil
	}

	level := ""

	var listPermission sql.NullString
	err = d.conn.QueryRowContext(ctx,
		`SELECT permission FROM list_shares WHERE list_id = ? AND shared_with_user_id = ?`, list.ID, userID,
	).Scan(&listPermission)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("querying list share for access check: %w", err)
	}
	if listPermission.Valid {
		level = listPermission.String
	}

	if list.CustomCategoryID != nil {
		var spacePermission sql.NullString
		err = d.conn.QueryRowContext(ctx,
			`SELECT permission FROM space_shares WHERE custom_category_id = ? AND shared_with_user_id = ?`,
			*list.CustomCategoryID, userID,
		).Scan(&spacePermission)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("querying space share for access check: %w", err)
		}
		if spacePermission.Valid && (level == "" || spacePermission.String == "write") {
			level = spacePermission.String
		}
	}

	return level, nil
}

// ListSharedListsForUser returns every List reachable via a list_shares or
// space_shares grant for userID, excluding any whose House they're already
// a plain member of (those already show up through the ordinary
// House-scoped ListListsForUser, so surfacing them again here would just be
// noise) — this is what backs the frontend's "Partagé avec moi" tab. Each
// result has AccessSource/AccessPermission populated; a List reachable both
// directly and via its Space keeps the higher ("write") permission and is
// only returned once.
func (d *DB) ListSharedListsForUser(ctx context.Context, userID int64) ([]*models.List, error) {
	// listSelect can't be reused as a prefix here the way every other query
	// in this package does: its SELECT column list is already closed before
	// its trailing FROM/JOIN clause, so the two extra columns this query
	// needs (shared.permission, shared.source) have to be spelled out in a
	// full column list of their own instead.
	query := `
		SELECT lists.id, lists.house_id, lists.name, lists.type, lists.icon, lists.created_at, lists.updated_at, lists.custom_category_id,
		       cc.id, cc.user_id, cc.name, cc.icon, cc.color, cc.position, cc.created_at,
		       shared.permission, shared.source
		FROM lists
		LEFT JOIN custom_categories cc ON cc.id = lists.custom_category_id
		JOIN (
			SELECT ls.list_id AS list_id, ls.permission AS permission, 'list_share' AS source
			FROM list_shares ls WHERE ls.shared_with_user_id = ?
			UNION ALL
			SELECT l.id AS list_id, ss.permission AS permission, 'space_share' AS source
			FROM space_shares ss JOIN lists l ON l.custom_category_id = ss.custom_category_id
			WHERE ss.shared_with_user_id = ?
		) shared ON shared.list_id = lists.id
		WHERE lists.house_id NOT IN (SELECT house_id FROM house_members WHERE user_id = ?)
		ORDER BY lists.created_at DESC`

	rows, err := d.conn.QueryContext(ctx, query, userID, userID, userID)
	if err != nil {
		return nil, fmt.Errorf("querying shared lists for user %d: %w", userID, err)
	}
	defer rows.Close()

	lists := []*models.List{}
	indexByID := map[int64]int{}
	for rows.Next() {
		l := &models.List{}
		var customCategoryID sql.NullInt64
		var catID sql.NullInt64
		var catUserID sql.NullInt64
		var catName, catIcon, catColor, catCreatedAt sql.NullString
		var catPosition sql.NullInt64
		var permission, source string
		if err := rows.Scan(&l.ID, &l.HouseID, &l.Name, &l.Type, &l.Icon, &l.CreatedAt, &l.UpdatedAt, &customCategoryID,
			&catID, &catUserID, &catName, &catIcon, &catColor, &catPosition, &catCreatedAt, &permission, &source); err != nil {
			return nil, fmt.Errorf("scanning shared list row: %w", err)
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
		l.AccessSource = source
		l.AccessPermission = permission

		if idx, ok := indexByID[l.ID]; ok {
			if l.AccessPermission == "write" {
				lists[idx].AccessPermission = "write"
			}
			continue
		}
		indexByID[l.ID] = len(lists)
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating shared list rows: %w", err)
	}
	return lists, nil
}
