// Package db owns all SQLite access: connection setup, schema application,
// and parameterized queries for lists and items. No other package imports
// database/sql directly.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO
)

//go:embed schema.sql
var schemaFS embed.FS

// DB wraps a *sql.DB configured for Trakka's usage pattern.
type DB struct {
	conn *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path, applies
// pragmas suited for a single-writer embedded workload, and applies the
// schema idempotently.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)",
		path,
	)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite: %w", err)
	}

	// SQLite allows only one writer at a time, and the pure-Go driver gains
	// nothing from pooling multiple readers here; a single shared
	// connection avoids SQLITE_BUSY errors under concurrent requests.
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("pinging sqlite: %w", err)
	}

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("reading embedded schema: %w", err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	if err := addColumnIfMissing(conn, "items", "price", "REAL"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "price_auto", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "image_url", "TEXT"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "target_month", "TEXT"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	// Must run after the ALTER TABLE above, same reasoning as
	// idx_lists_house_id below: the column doesn't exist yet on a database
	// that predates this migration.
	if _, err := conn.Exec(`CREATE INDEX IF NOT EXISTS idx_items_target_month ON items(target_month)`); err != nil {
		return nil, fmt.Errorf("creating items.target_month index: %w", err)
	}

	if err := addColumnIfMissing(conn, "lists", "house_id", "INTEGER REFERENCES houses(id) ON DELETE CASCADE"); err != nil {
		return nil, fmt.Errorf("migrating lists table: %w", err)
	}
	// Must run after the ALTER TABLE above: on a truly fresh database the
	// lists table (created by schema.sql, without house_id) wouldn't have
	// this column yet, and CREATE INDEX would fail with "no such column".
	if _, err := conn.Exec(`CREATE INDEX IF NOT EXISTS idx_lists_house_id ON lists(house_id)`); err != nil {
		return nil, fmt.Errorf("creating lists.house_id index: %w", err)
	}
	if err := ensureDefaultHouse(conn); err != nil {
		return nil, fmt.Errorf("seeding default house: %w", err)
	}

	return &DB{conn: conn}, nil
}

// ensureDefaultHouse guarantees at least one house always exists and that
// every list belongs to one, so that "house_id" can be treated as required
// at the application level even though the column itself is nullable (added
// via ALTER TABLE, which cannot retroactively enforce NOT NULL). It creates
// a default house named "Maison Principale" the first time no house exists
// at all (a fresh database, or an existing one migrating from before houses
// existed), and reassigns any list still missing a house_id — e.g. rows
// created before this migration — to the oldest house. Runs on every
// startup; a no-op once every list already has a house.
func ensureDefaultHouse(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM houses`).Scan(&count); err != nil {
		return fmt.Errorf("counting houses: %w", err)
	}
	if count == 0 {
		if _, err := conn.Exec(`INSERT INTO houses (name) VALUES ('Maison Principale')`); err != nil {
			return fmt.Errorf("inserting default house: %w", err)
		}
	}

	if _, err := conn.Exec(
		`UPDATE lists SET house_id = (SELECT id FROM houses ORDER BY id ASC LIMIT 1) WHERE house_id IS NULL`,
	); err != nil {
		return fmt.Errorf("backfilling lists.house_id: %w", err)
	}
	return nil
}

// addColumnIfMissing adds a column to an existing table if it isn't already
// there. SQLite has no `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, so this
// guards the ALTER in Go via PRAGMA table_info instead — see "Evolving the
// schema" in docs/DATABASE.md. table/column/sqlType are always hard-coded
// call-site constants, never user input, so building the statements with
// fmt.Sprintf is safe (SQLite has no placeholder syntax for identifiers).
func addColumnIfMissing(conn *sql.DB, table, column, sqlType string) error {
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("reading table_info(%s): %w", table, err)
	}
	defer rows.Close()

	// PRAGMA table_info columns: cid, name, type, notnull, dflt_value, pk.
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, colType string
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scanning table_info(%s) row: %w", table, err)
		}
		if name == column {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info(%s): %w", table, err)
	}
	if found {
		return nil
	}

	if _, err := conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, sqlType)); err != nil {
		return fmt.Errorf("adding column %s.%s: %w", table, column, err)
	}
	return nil
}

// Close releases the underlying database connection. Safe to call once
// during graceful shutdown.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Ping checks that the database is reachable; used by the /healthz handler.
func (d *DB) Ping(ctx context.Context) error {
	return d.conn.PingContext(ctx)
}
