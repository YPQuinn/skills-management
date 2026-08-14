package state

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestOpenAppliesAndReopensMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 8 {
		t.Fatalf("schema version after open: got %d, want 8", got)
	}
	db.Close()

	// reopening the same database must not re-apply or error
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 8 {
		t.Fatalf("schema version after reopen: got %d, want 8", got)
	}
	db.Close()
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (42);`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = Open(path)
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("reopening a newer database: got %v, want a newer-schema error", err)
	}
}

func TestMigrationFailureRollsBack(t *testing.T) {
	broken := fstest.MapFS{
		"migrations/001_test.sql": {Data: []byte("CREATE TABLE t1 (id INTEGER PRIMARY KEY);")},
		"migrations/002_test.sql": {Data: []byte("CREATE TABLE t2 (id INTEGER PRIMARY KEY); THIS IS NOT SQL;")},
	}
	db, err := openDB(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := migrate(db, broken); err == nil {
		t.Fatal("migrate with a broken migration: want error, got nil")
	}
	if got := appliedVersion(t, db); got != 1 {
		t.Fatalf("version after failed migration: got %d, want 1 (rolled back)", got)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='t2';`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("table from the failed migration survived the rollback")
	}

	// the same database then applies the fixed migration set cleanly
	broken["migrations/002_test.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE t2 (id INTEGER PRIMARY KEY);")}
	if err := migrate(db, broken); err != nil {
		t.Fatalf("migrate after fixing migration 002: %v", err)
	}
	if got := appliedVersion(t, db); got != 2 {
		t.Fatalf("version after fixed migration: got %d, want 2", got)
	}
}

func TestMigrationGapsRejected(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/001_test.sql": {Data: []byte("CREATE TABLE t1 (id INTEGER PRIMARY KEY);")},
		"migrations/003_test.sql": {Data: []byte("CREATE TABLE t3 (id INTEGER PRIMARY KEY);")},
	}
	db, err := openDB(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrate(db, fsys); err == nil {
		t.Fatal("migrate with a version gap: want error, got nil")
	}
}

func TestPragmas(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys;`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys: got %d, want 1", fk)
	}

	var jm string
	if err := db.QueryRow(`PRAGMA journal_mode;`).Scan(&jm); err != nil {
		t.Fatal(err)
	}
	if jm != "wal" {
		t.Fatalf("journal_mode: got %q, want wal", jm)
	}

	var bt int
	if err := db.QueryRow(`PRAGMA busy_timeout;`).Scan(&bt); err != nil {
		t.Fatal(err)
	}
	if bt != 5000 {
		t.Fatalf("busy_timeout: got %d, want 5000", bt)
	}
}

func TestInspectSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := InspectSchema(path); err != nil || got != 8 {
		t.Fatalf("InspectSchema: got %d, %v", got, err)
	}
	db.Close()

	// inspection is read-only: it must not change the database file
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := InspectSchema(path); err != nil || got != 8 {
		t.Fatalf("InspectSchema after close: got %d, %v", got, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("InspectSchema modified the state database")
	}
}

func TestInspectSchemaErrors(t *testing.T) {
	// missing file
	if _, err := InspectSchema(filepath.Join(t.TempDir(), "nope.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: got %v, want os.ErrNotExist", err)
	}

	// newer schema is rejected without migrating
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (42);`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := InspectSchema(path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("newer schema: got %v, want a newer-schema error", err)
	}

	// corrupt file
	corrupt := filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(corrupt, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectSchema(corrupt); err == nil {
		t.Fatal("corrupt file: want error, got nil")
	}

	// a valid but never-migrated database reports version 0
	empty := filepath.Join(t.TempDir(), "empty.db")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectSchema(empty); err != nil || got != 0 {
		t.Fatalf("unmigrated database: got %d, %v", got, err)
	}
}

func appliedVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}
