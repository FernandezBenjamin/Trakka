package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"trakka/internal/models"
)

// timeFormat matches the strftime('%Y-%m-%dT%H:%M:%fZ', 'now') shape used
// by every timestamp column across internal/db/migrations/, keeping
// expires_at lexicographically comparable to strftime('now') in SQL.
const timeFormat = "2006-01-02T15:04:05.000Z"

// CreateSession inserts a new session keyed by tokenHash (the SHA-256 hash
// of the raw cookie token — never the raw token itself).
func (d *DB) CreateSession(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash, userID, expiresAt.UTC().Format(timeFormat))
	if err != nil {
		return fmt.Errorf("inserting session: %w", err)
	}
	return nil
}

// GetSessionByHash fetches a session by its token hash. Returns ErrNotFound
// if no such session exists or if it has already expired.
func (d *DB) GetSessionByHash(ctx context.Context, tokenHash string) (*models.Session, error) {
	s := &models.Session{}
	err := d.conn.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, created_at FROM sessions
		 WHERE id = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`, tokenHash,
	).Scan(&s.ID, &s.UserID, &s.ExpiresAt, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying session: %w", err)
	}
	return s, nil
}

// DeleteSessionByHash removes a session (logout). Returns ErrNotFound if no
// such session exists.
func (d *DB) DeleteSessionByHash(ctx context.Context, tokenHash string) error {
	res, err := d.conn.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, tokenHash)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reading rows affected for session delete: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
