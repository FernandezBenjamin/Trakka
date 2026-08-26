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
func (d *DB) CreateUser(ctx context.Context, email string, passwordHash, oidcSubject, oidcIssuer *string, displayName string) (*models.User, error) {
	res, err := d.conn.ExecContext(ctx,
		`INSERT INTO users (email, password_hash, oidc_subject, oidc_issuer, display_name) VALUES (?, ?, ?, ?, ?)`,
		email, passwordHash, oidcSubject, oidcIssuer, displayName)
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
	return d.GetUser(ctx, id)
}

// GetUser fetches a single user's public fields by id. Returns ErrNotFound
// if no such user exists.
func (d *DB) GetUser(ctx context.Context, id int64) (*models.User, error) {
	u := &models.User{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, email, display_name, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user %d: %w", id, err)
	}
	return u, nil
}

// GetUserByEmail fetches a user (with credentials, for authentication) by
// email, compared case-insensitively. Returns ErrNotFound if no such user
// exists.
func (d *DB) GetUserByEmail(ctx context.Context, email string) (*models.UserWithCredentials, error) {
	return d.getUserWithCredentials(ctx,
		`SELECT id, email, display_name, created_at, password_hash, oidc_subject, oidc_issuer
		 FROM users WHERE email = ?`, email)
}

// GetUserByOIDCSubject fetches a user (with credentials) by their OIDC
// identity (issuer + subject). Returns ErrNotFound if no such user exists.
func (d *DB) GetUserByOIDCSubject(ctx context.Context, issuer, subject string) (*models.UserWithCredentials, error) {
	return d.getUserWithCredentials(ctx,
		`SELECT id, email, display_name, created_at, password_hash, oidc_subject, oidc_issuer
		 FROM users WHERE oidc_issuer = ? AND oidc_subject = ?`, issuer, subject)
}

func (d *DB) getUserWithCredentials(ctx context.Context, query string, args ...any) (*models.UserWithCredentials, error) {
	u := &models.UserWithCredentials{}
	err := d.conn.QueryRowContext(ctx, query, args...).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.PasswordHash, &u.OIDCSubject, &u.OIDCIssuer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}
	return u, nil
}
