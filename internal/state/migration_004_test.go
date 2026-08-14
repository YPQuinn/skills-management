package state

import (
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// legacyFS returns a migration set containing only 001-003, the schema an
// earlier build left on disk.
func legacyFS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := &fstest.MapFS{}
	for _, name := range []string{"001_initial.sql", "002_sources.sql", "003_source_observation.sql"} {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		(*fsys)["migrations/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigration004UniqueNames is the upgrade path: a database created by an
// earlier build (schema version 3) may hold duplicate Source names. Opening
// it with the current executable applies migration 004, which renames later
// duplicates deterministically and enforces uniqueness from then on.
func TestMigration004UniqueNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, legacyFS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 3 {
		t.Fatalf("legacy schema: got %d, want 3", got)
	}

	// v3 allowed duplicate names; insert them the way the old binary could
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, name := range []string{"dup", "dup", "other", "dup"} {
		if _, err := db.Exec(`INSERT INTO sources
			(kind, location, ref, subpath, name, created_at, updated_at, available)
			VALUES ('local', ?, '', '', ?, ?, ?, 1)`,
			"/tmp/m"+string(rune('0'+i)), name, now, now); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	// reopening runs migration 004: the earliest id keeps each name and
	// later duplicates are suffixed with their id
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := appliedVersion(t, db); got != 8 {
		t.Fatalf("schema after upgrade: got %d, want 8", got)
	}
	rows, err := db.Query(`SELECT id, name FROM sources ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"dup", "dup-2", "other", "dup-4"}
	if len(got) != len(want) {
		t.Fatalf("names after upgrade: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names after upgrade: got %v, want %v", got, want)
		}
	}

	// the unique index now refuses a new duplicate name
	if _, err := db.Exec(`INSERT INTO sources
		(kind, location, ref, subpath, name, created_at, updated_at, available)
		VALUES ('local', '/tmp/dup-again', '', '', 'dup', ?, ?, 1)`, now, now); err == nil || !IsUniqueViolation(err) {
		t.Fatalf("duplicate name after upgrade: got %v, want a unique violation", err)
	}

	// the read path works on a renamed row
	gotSource, err := GetSource(db, 2)
	if err != nil {
		t.Fatal(err)
	}
	if gotSource.Name != "dup-2" {
		t.Fatalf("renamed row read: got %q", gotSource.Name)
	}
}

// TestMigration004FailsLoudlyOnCollision proves a residual collision with an
// existing name that already equals a suffixed form fails the migration
// instead of guessing.
func TestMigration004FailsLoudlyOnCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, legacyFS(t)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, name := range []string{"dup", "dup", "dup-2"} {
		if _, err := db.Exec(`INSERT INTO sources
			(kind, location, ref, subpath, name, created_at, updated_at, available)
			VALUES ('local', ?, '', '', ?, ?, ?, 1)`,
			"/tmp/c"+string(rune('0'+i)), name, now, now); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	if _, err := Open(path); err == nil {
		t.Fatal("colliding suffixed name: want a migration error")
	}
}
