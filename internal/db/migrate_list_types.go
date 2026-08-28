package db

import (
	"database/sql"
	"fmt"
)

// applyListTypesWidenMigration is migration 6 ("Create different type of
// buy list"), kept as Go code rather than a migrations/0006_*.sql file
// because SQLite has no `ALTER TABLE ... ADD/DROP CONSTRAINT`: widening the
// lists.type CHECK constraint (from ('shopping', 'todo') to add 'groceries',
// 'recurring_shopping', and 'custom') means rebuilding the whole table, and
// that rebuild needs `PRAGMA foreign_keys = OFF` set *outside* any
// transaction — SQLite ignores changes to that pragma once a transaction is
// already open — which sqlMigration's single BEGIN/COMMIT around one script
// (in migrate.go) can't express.
//
// This follows SQLite's own documented 12-step "Making Other Kinds Of Table
// Schema Changes" procedure; see "Evolving the schema" in docs/DATABASE.md
// for the full reasoning, including why the old table is DROPped (never
// RENAMEd) and the new one is built under a temporary name first — renaming
// "lists" itself, even briefly, makes SQLite rewrite items.list_id's
// foreign key definition to point at whatever temporary name it was renamed
// to, which was confirmed to actually happen when this migration was first
// written (as a runtime check in the pre-versioning code this replaced).
//
// Unlike that original code, this never needs to detect whether the
// migration has already run: PRAGMA user_version guarantees this function
// is called at most once per database, ever, so there's no need to inspect
// sqlite_master's stored SQL for the table first.
func applyListTypesWidenMigration(conn *sql.DB) error {
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
	if _, err := tx.Exec(`CREATE INDEX idx_lists_type ON lists(type)`); err != nil {
		return fmt.Errorf("recreating idx_lists_type: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX idx_lists_house_id ON lists(house_id)`); err != nil {
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

	if err := setSchemaVersion(tx, 6); err != nil {
		return err
	}
	return tx.Commit()
}
