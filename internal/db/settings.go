package db

import (
	"context"
	"fmt"
)

// GetAllSettings returns every row in system_settings as a key->value map —
// the raw material internal/settings.Resolve merges against env-var
// defaults. A key that was never set simply isn't in the map; that's a
// normal, valid result (e.g. on a fresh database), not an error.
func (d *DB) GetAllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := d.conn.QueryContext(ctx, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, fmt.Errorf("querying system_settings: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scanning system_settings row: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating system_settings rows: %w", err)
	}
	return out, nil
}

// SetSettings upserts every key/value pair in updates atomically, inside a
// single transaction — a PATCH to /api/v1/admin/settings can touch several
// keys at once (e.g. instance_name and every oidc_* field together), and
// this guarantees the table is never left with only some of them applied if
// something in the batch fails partway through.
func (d *DB) SetSettings(ctx context.Context, updates map[string]string) error {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning settings update transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once committed

	for key, value := range updates {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, value,
		); err != nil {
			return fmt.Errorf("upserting system_settings %q: %w", key, err)
		}
	}
	return tx.Commit()
}
