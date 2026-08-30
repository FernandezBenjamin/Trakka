package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"trakka/internal/models"
)

// CreateUser inserts a new user and returns the persisted, API-facing row.
// Exactly one of passwordHash or (oidcSubject, oidcIssuer) is expected to
// be non-nil, per the users table's CHECK constraint. Returns
// ErrDuplicateEmail if the email is already registered.
//
// The very first account ever created (local or OIDC-provisioned) is made
// an admin automatically: Trakka has no separate seeding mechanism or CLI
// (it is env-var-configured only, see CLAUDE.md), so this is the only way
// an instance ever gets an initial administrator. The count-then-insert
// check runs inside a transaction so two concurrent registrations against a
// brand new database can't both see count == 0 and both become admin.
func (d *DB) CreateUser(ctx context.Context, email string, passwordHash, oidcSubject, oidcIssuer *string, displayName string) (*models.User, error) {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning user creation transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return nil, fmt.Errorf("counting users: %w", err)
	}
	isAdmin := count == 0

	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, oidc_subject, oidc_issuer, display_name, is_admin) VALUES (?, ?, ?, ?, ?, ?)`,
		email, passwordHash, oidcSubject, oidcIssuer, displayName, boolToInt(isAdmin))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ErrDuplicateEmail
		}
		return nil, fmt.Errorf("inserting user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("reading inserted user id: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing user creation: %w", err)
	}
	return d.GetUser(ctx, id)
}

// GetUser fetches a single user's public fields by id. Returns ErrNotFound
// if no such user exists.
func (d *DB) GetUser(ctx context.Context, id int64) (*models.User, error) {
	u := &models.User{}
	var isAdmin, keepLastPage int
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, email, display_name, created_at, is_admin, keep_last_page FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &isAdmin, &keepLastPage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user %d: %w", id, err)
	}
	u.IsAdmin = isAdmin != 0
	u.KeepLastPage = keepLastPage != 0
	return u, nil
}

// UpdateUserKeepLastPage sets whether the frontend should reopen on the
// user's last-visited dashboard tab/list (see models.User.KeepLastPage).
// Returns ErrNotFound if no such user exists.
func (d *DB) UpdateUserKeepLastPage(ctx context.Context, id int64, keepLastPage bool) (*models.User, error) {
	res, err := d.conn.ExecContext(ctx,
		`UPDATE users SET keep_last_page = ? WHERE id = ?`, boolToInt(keepLastPage), id)
	if err != nil {
		return nil, fmt.Errorf("updating user %d keep_last_page: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("reading rows affected updating user %d: %w", id, err)
	}
	if affected == 0 {
		return nil, ErrNotFound
	}
	return d.GetUser(ctx, id)
}

// GetUserByEmail fetches a user (with credentials, for authentication) by
// email, compared case-insensitively. Returns ErrNotFound if no such user
// exists.
func (d *DB) GetUserByEmail(ctx context.Context, email string) (*models.UserWithCredentials, error) {
	return d.getUserWithCredentials(ctx,
		`SELECT id, email, display_name, created_at, is_admin, keep_last_page, password_hash, oidc_subject, oidc_issuer
		 FROM users WHERE email = ?`, email)
}

// GetUserByOIDCSubject fetches a user (with credentials) by their OIDC
// identity (issuer + subject). Returns ErrNotFound if no such user exists.
func (d *DB) GetUserByOIDCSubject(ctx context.Context, issuer, subject string) (*models.UserWithCredentials, error) {
	return d.getUserWithCredentials(ctx,
		`SELECT id, email, display_name, created_at, is_admin, keep_last_page, password_hash, oidc_subject, oidc_issuer
		 FROM users WHERE oidc_issuer = ? AND oidc_subject = ?`, issuer, subject)
}

func (d *DB) getUserWithCredentials(ctx context.Context, query string, args ...any) (*models.UserWithCredentials, error) {
	u := &models.UserWithCredentials{}
	var isAdmin, keepLastPage int
	err := d.conn.QueryRowContext(ctx, query, args...).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &isAdmin, &keepLastPage, &u.PasswordHash, &u.OIDCSubject, &u.OIDCIssuer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	u.IsAdmin = isAdmin != 0
	u.KeepLastPage = keepLastPage != 0
	return u, nil
}
