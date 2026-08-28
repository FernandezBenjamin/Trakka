// Package db owns all SQLite access: connection setup, schema application,
// and parameterized queries for lists and items. No other package imports
// database/sql directly.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"

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
	if err := addColumnIfMissing(conn, "items", "due_date", "TEXT"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "is_recurring", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "recurrence_rule", "TEXT"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "recurrence_end_date", "TEXT"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}
	if err := addColumnIfMissing(conn, "items", "is_urgent", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("migrating items table: %w", err)
	}

	if err := addColumnIfMissing(conn, "users", "is_admin", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return nil, fmt.Errorf("migrating users table: %w", err)
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
	if err := migrateListsTypeCheck(conn); err != nil {
		return nil, fmt.Errorf("migrating lists.type constraint: %w", err)
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

// migrateListsTypeCheck widens the `type` CHECK constraint on an existing
// lists table from the original ('shopping', 'todo') to the current full
// set ('todo', 'shopping', 'groceries', 'recurring_shopping', 'custom') —
// see the comment above the `lists` CREATE TABLE statement in schema.sql
// for why editing that statement alone isn't enough: SQLite has no
// `ALTER TABLE ... ADD/DROP CONSTRAINT`, so a database file created before
// this migration existed keeps enforcing the old, narrower CHECK forever
// unless something rebuilds the table. This is that something, run
// unconditionally at every startup right after lists.house_id is in place
// (the recreated table needs to carry that column too). It detects whether
// the migration is still needed by inspecting sqlite_master's stored SQL
// for the table — a fresh database created after this migration was added
// already has the wide CHECK from schema.sql itself, so this is a no-op for
// it — and otherwise rebuilds the table following SQLite's own documented
// 12-step "Making Other Kinds Of Table Schema Changes" procedure: create a
// new table under a temporary name, copy every row across preserving its
// id, drop the *old* table, then rename the temporary one into the now-free
// "lists" name — deliberately in that order (new table first, old one
// dropped rather than renamed away) rather than the more obvious
// "rename old table out of the way, create the new one in its place":
// renaming "lists" itself, even briefly, makes SQLite rewrite
// items.list_id's foreign key definition to point at whatever temporary
// name "lists" was renamed to (this was tried and confirmed to actually
// happen — PRAGMA foreign_keys=OFF does *not* suppress it, only
// PRAGMA legacy_alter_table would, and even then only by accident), leaving
// a dangling reference once that temporary table is dropped. Dropping the
// old table outright never triggers that rewrite (only RENAME does), and
// the final rename target ("lists") has nothing pointing at its temporary
// pre-rename name, so items.list_id's definition — untouched throughout —
// simply resolves correctly again the moment the name "lists" exists.
// PRAGMA foreign_keys=OFF around the whole thing (set *outside* the
// transaction, since SQLite ignores changes to that pragma mid-transaction)
// is still worth keeping: it's what stops SQLite from enforcing
// items.list_id against a temporarily-absent "lists" table while the DROP/
// CREATE/RENAME sequence is mid-flight.
func migrateListsTypeCheck(conn *sql.DB) error {
	var tableSQL string
	err := conn.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'lists'`).Scan(&tableSQL)
	if err == sql.ErrNoRows {
		return nil // schema.sql hasn't created it yet somehow — nothing to migrate
	}
	if err != nil {
		return fmt.Errorf("reading lists table definition: %w", err)
	}
	if strings.Contains(tableSQL, "'groceries'") {
		return nil // already has the wide CHECK, nothing to do
	}

	if _, err := conn.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disabling foreign keys for lists migration: %w", err)
	}
	defer func() { _, _ = conn.Exec(`PRAGMA foreign_keys = ON`) }()

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning lists migration transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE lists_type_widen_new (
		    id         INTEGER PRIMARY KEY AUTOINCREMENT,
		    name       TEXT NOT NULL,
		    type       TEXT NOT NULL DEFAULT 'shopping' CHECK (type IN ('todo', 'shopping', 'groceries', 'recurring_shopping', 'custom')),
		    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		    house_id   INTEGER REFERENCES houses(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("creating replacement lists table: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO lists_type_widen_new (id, name, type, created_at, updated_at, house_id)
		SELECT id, name, type, created_at, updated_at, house_id FROM lists
	`); err != nil {
		return fmt.Errorf("copying lists rows: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE lists`); err != nil {
		return fmt.Errorf("dropping old lists table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE lists_type_widen_new RENAME TO lists`); err != nil {
		return fmt.Errorf("renaming replacement lists table into place: %w", err)
	}
	// The old table's indexes were attached to it and went away along with
	// it (DROP TABLE drops its indexes too) — recreate them so the rebuilt
	// table isn't left missing indexes it had a moment ago.
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(type)`); err != nil {
		return fmt.Errorf("recreating idx_lists_type: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_lists_house_id ON lists(house_id)`); err != nil {
		return fmt.Errorf("recreating idx_lists_house_id: %w", err)
	}

	// Belt-and-suspenders: confirm items.list_id (the only foreign key that
	// references lists) actually still resolves before committing, rather
	// than trusting the reasoning above blindly.
	rows, err := tx.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("running foreign_key_check after lists migration: %w", err)
	}
	hasViolation := rows.Next()
	closeErr := rows.Close()
	if hasViolation {
		return fmt.Errorf("lists migration left a dangling foreign key reference")
	}
	if closeErr != nil {
		return fmt.Errorf("checking foreign_key_check results: %w", closeErr)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing lists migration: %w", err)
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
