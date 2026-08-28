package db

import (
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestLoadMigrationsSequential(t *testing.T) {
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected at least one migration")
	}
	for i, m := range migrations {
		if m.version != i+1 {
			t.Fatalf("migration at index %d has version %d, want %d", i, m.version, i+1)
		}
	}
	// Migration 6 (widening lists.type's CHECK constraint) is the one
	// documented exception that must run as Go code, not an embedded .sql
	// file — see migrate_list_types.go.
	if migrations[5].name != "list_types_widen" {
		t.Fatalf("expected migration 6 to be list_types_widen, got %q", migrations[5].name)
	}
}

// TestMigrateFreshDatabase confirms a brand new database, migrated from
// scratch, ends up fully up to date and with a schema that actually works —
// including the lists.type CHECK constraint having already been widened by
// migration 6 by the time this returns, since a fresh install applies every
// migration in order rather than starting from the pre-widened shape.
func TestMigrateFreshDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trakka.db")
	d, err := Open(dbPath, discardLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	var version int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	latest := migrations[len(migrations)-1].version
	if version != latest {
		t.Fatalf("user_version = %d, want %d (latest migration)", version, latest)
	}

	for _, table := range []string{
		"houses", "lists", "items", "users", "sessions", "house_members",
		"price_alerts", "custom_categories", "system_settings", "space_shares", "list_shares",
	} {
		var name string
		err := d.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist after migration: %v", table, err)
		}
	}

	// A default house should have been seeded by ensureDefaultHouse, run
	// right after migrate() inside Open().
	var houseCount int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM houses`).Scan(&houseCount); err != nil {
		t.Fatalf("counting houses: %v", err)
	}
	if houseCount == 0 {
		t.Fatal("expected ensureDefaultHouse to have seeded at least one house")
	}

	// The lists.type CHECK constraint must already be the widened set from
	// migration 6, not the original ('shopping', 'todo') pair from
	// migration 1 — proves the migrations actually ran in order rather than
	// e.g. migration 6 being skipped.
	if _, err := d.conn.Exec(`INSERT INTO lists (name, type, house_id) VALUES ('Test', 'groceries', (SELECT id FROM houses LIMIT 1))`); err != nil {
		t.Fatalf("inserting a 'groceries' list should succeed once migration 6 has widened the CHECK constraint: %v", err)
	}
}

// TestMigrateAdoptsPreExistingDatabase simulates a database that was
// already in full use before this migration system existed: every table
// this package's migrations would eventually create, but PRAGMA
// user_version still at its default of 0 (nothing has ever set it). Open()
// must adopt it directly at the latest version without attempting to
// re-run any migration SQL — which would otherwise fail outright (e.g.
// "duplicate column name") — and, critically, without touching any
// existing data.
func TestMigrateAdoptsPreExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trakka.db")

	seed, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("opening seed connection: %v", err)
	}
	if _, err := seed.Exec(`CREATE TABLE houses (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("seeding houses table: %v", err)
	}
	// ensureDefaultHouse (run by Open() right after migrate()) always
	// touches lists.house_id, so a realistic pre-existing database needs at
	// least this much of the real shape, not just houses alone.
	if _, err := seed.Exec(`CREATE TABLE lists (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, house_id INTEGER)`); err != nil {
		t.Fatalf("seeding lists table: %v", err)
	}
	if _, err := seed.Exec(`INSERT INTO houses (id, name) VALUES (1, 'Pre-existing House')`); err != nil {
		t.Fatalf("seeding a house row: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("closing seed connection: %v", err)
	}

	d, err := Open(dbPath, discardLogger())
	if err != nil {
		t.Fatalf("Open on a pre-existing database should adopt it, not fail: %v", err)
	}
	defer d.Close()

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	latest := migrations[len(migrations)-1].version

	var version int
	if err := d.conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("reading user_version: %v", err)
	}
	if version != latest {
		t.Fatalf("user_version = %d, want %d (adopted at latest)", version, latest)
	}

	var name string
	if err := d.conn.QueryRow(`SELECT name FROM houses WHERE id = 1`).Scan(&name); err != nil {
		t.Fatalf("pre-existing house row should survive adoption: %v", err)
	}
	if name != "Pre-existing House" {
		t.Fatalf("house name = %q, want unchanged %q", name, "Pre-existing House")
	}

	// No backup should have been taken for this path: nothing about
	// adoption modifies the schema, so there's nothing to protect against.
	if _, err := os.Stat(filepath.Join(filepath.Dir(dbPath), "backups")); !os.IsNotExist(err) {
		t.Fatalf("expected no backups/ directory for a pure adoption, stat error: %v", err)
	}
}

// TestBackupBeforeMigrationCreatesSnapshot exercises backupBeforeMigration
// directly: it should create backups/<name>.db next to dbPath, and that
// snapshot should contain the data that existed at backup time.
func TestBackupBeforeMigrationCreatesSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trakka.db")
	conn, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("opening connection: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO t (v) VALUES ('before-migration')`); err != nil {
		t.Fatalf("inserting row: %v", err)
	}

	if err := backupBeforeMigration(conn, dbPath, 3, 4, discardLogger()); err != nil {
		t.Fatalf("backupBeforeMigration: %v", err)
	}

	backupDir := filepath.Join(filepath.Dir(dbPath), "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("reading backup directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one backup file, got %d", len(entries))
	}

	backupConn, err := sql.Open("sqlite", "file:"+filepath.Join(backupDir, entries[0].Name())+"?mode=ro")
	if err != nil {
		t.Fatalf("opening backup file: %v", err)
	}
	defer backupConn.Close()

	var v string
	if err := backupConn.QueryRow(`SELECT v FROM t WHERE id = 1`).Scan(&v); err != nil {
		t.Fatalf("reading row from backup: %v", err)
	}
	if v != "before-migration" {
		t.Fatalf("backup row v = %q, want %q", v, "before-migration")
	}
}

func TestHasExistingSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trakka.db")
	conn, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("opening connection: %v", err)
	}
	defer conn.Close()

	legacy, err := hasExistingSchema(conn)
	if err != nil {
		t.Fatalf("hasExistingSchema on empty db: %v", err)
	}
	if legacy {
		t.Fatal("a genuinely empty database should not be reported as having an existing schema")
	}

	if _, err := conn.Exec(`CREATE TABLE anything (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	legacy, err = hasExistingSchema(conn)
	if err != nil {
		t.Fatalf("hasExistingSchema after creating a table: %v", err)
	}
	if !legacy {
		t.Fatal("a database with a user table should be reported as having an existing schema")
	}
}
