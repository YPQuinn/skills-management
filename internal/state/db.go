// Package state is the only package that executes SQL. It opens the SQLite
// state database with foreign keys, WAL, and a finite busy timeout, and
// applies numbered SQL migrations embedded in the executable, each in its
// own transaction.
package state

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const dsnPragmas = "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"

// inspectDSNPragmas opens an existing database without creating or migrating
// it: busy_timeout only adjusts this connection and query_only prevents every
// write, so InspectSchema never touches the file.
const inspectDSNPragmas = "?_pragma=busy_timeout(5000)&_pragma=query_only(ON)"

// Open opens (creating if needed) the state database at path and brings it
// to the schema version this executable knows. A database with a newer
// schema is rejected instead of being touched.
func Open(path string) (*sql.DB, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db, migrationsFS); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func openDB(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path+dsnPragmas)
}

type migration struct {
	version int
	file    string
	sql     string
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, ok := parseVersion(e.Name())
		if !ok {
			return nil, fmt.Errorf("migration %q: filename must start with a zero-padded number", e.Name())
		}
		content, err := fs.ReadFile(fsys, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: version, file: e.Name(), sql: string(content)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration versions must be contiguous, got %d at position %d", m.version, i+1)
		}
	}
	return ms, nil
}

func parseVersion(name string) (int, bool) {
	n := strings.TrimSuffix(name, ".sql")
	i := strings.Index(n, "_")
	if i <= 0 {
		return 0, false
	}
	v, err := strconv.Atoi(n[:i])
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// InspectSchema reports the schema version recorded in the existing state
// database at path without creating, migrating, or writing the file. A
// missing file is reported through os.ErrNotExist. A database that is not
// readable SQLite, and one whose schema version is newer than this
// executable supports, are reported as errors; older and current versions
// are returned as-is and Open migrates them on first use.
func InspectSchema(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	db, err := sql.Open("sqlite", path+inspectDSNPragmas)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var hasMigrations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations';`).Scan(&hasMigrations); err != nil {
		return 0, err
	}
	if hasMigrations == 0 {
		return 0, nil // valid but never migrated; Open will migrate it
	}
	var applied int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&applied); err != nil {
		return 0, err
	}
	ms, err := loadMigrations(migrationsFS)
	if err != nil {
		return 0, err
	}
	if applied > len(ms) {
		return 0, fmt.Errorf("state database schema version %d is newer than this executable supports (%d)", applied, len(ms))
	}
	return applied, nil
}

// migrate applies every embedded migration newer than the recorded schema
// version, each in one transaction together with its version record.
func migrate(db *sql.DB, fsys fs.FS) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);`); err != nil {
		return err
	}
	var applied int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations;`).Scan(&applied); err != nil {
		return err
	}
	ms, err := loadMigrations(fsys)
	if err != nil {
		return err
	}
	if applied > len(ms) {
		return fmt.Errorf("state database schema version %d is newer than this executable supports (%d)", applied, len(ms))
	}

	for _, m := range ms {
		if m.version <= applied {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %s: %w", m.file, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", m.file, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", m.file, err)
		}
	}
	return nil
}
