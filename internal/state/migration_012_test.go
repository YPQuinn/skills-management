package state

import (
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func v11FS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := &fstest.MapFS{}
	for _, name := range []string{
		"001_initial.sql", "002_sources.sql", "003_source_observation.sql",
		"004_unique_source_name.sql", "005_skills.sql", "006_terminal_receipts.sql",
		"007_groups_targets.sql", "008_sync.sql", "009_distribution.sql",
		"010_link_intent_slots.sql", "011_remove_ops.sql",
	} {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		(*fsys)["migrations/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigration012PreservesManagedLinks proves a version-11 database with
// an owned link upgrades by adding identity columns defaulting to 0,0
// without dropping the row.
func TestMigration012PreservesManagedLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v11FS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 11 {
		t.Fatalf("v11 schema: got %d, want 11", got)
	}
	now := time.Now().UTC()
	targetID, err := InsertTarget(db, Target{
		Name: "t1", Path: "/tmp/t", Adapter: "custom", Scope: "custom",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(testSourceID(t, db), 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO managed_links
		(target_id, skill_id, link_path, raw_target, established_at)
		VALUES (?, ?, ?, ?, ?)`,
		targetID, skillID, "/tmp/t/alpha", "/store/alpha", timeToSQL(&now)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := appliedVersion(t, db); got != 12 {
		t.Fatalf("migrated schema: got %d, want 12", got)
	}
	got, err := GetManagedLink(db, targetID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RawTarget != "/store/alpha" || got.LinkDev != 0 || got.LinkIno != 0 {
		t.Fatalf("preserved link: %+v", got)
	}
}
