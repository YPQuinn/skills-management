package state

import (
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func v9FS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := &fstest.MapFS{}
	for _, name := range []string{
		"001_initial.sql", "002_sources.sql", "003_source_observation.sql",
		"004_unique_source_name.sql", "005_skills.sql", "006_terminal_receipts.sql",
		"007_groups_targets.sql", "008_sync.sql", "009_distribution.sql",
	} {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		(*fsys)["migrations/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigration010LegacyOpenIntents proves a version-9 database with
// unfinished create and remove intents upgrades to slot_name=” and
// phase=planned, the legacy recovery inputs.
func TestMigration010LegacyOpenIntents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v9FS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 9 {
		t.Fatalf("v9 schema: got %d, want 9", got)
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
	if _, err := db.Exec(`INSERT INTO link_intents
		(target_id, skill_id, action, link_path, raw_target, created_at)
		VALUES (?, ?, 'create', ?, ?, ?)`,
		targetID, skillID, "/tmp/t/alpha", "/store/alpha", timeToSQL(&now)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO link_intents
		(target_id, skill_id, action, link_path, raw_target, created_at)
		VALUES (?, ?, 'remove', ?, ?, ?)`,
		targetID, skillID, "/tmp/t/alpha", "/store/alpha", timeToSQL(&now)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := appliedVersion(t, db); got != 10 {
		t.Fatalf("migrated schema: got %d, want 10", got)
	}
	intents, err := ListOpenLinkIntents(db)
	if err != nil || len(intents) != 2 {
		t.Fatalf("legacy intents: %+v, %v", intents, err)
	}
	for _, it := range intents {
		if it.SlotName != "" || it.Phase != LinkPhasePlanned {
			t.Fatalf("migrated intent: %+v", it)
		}
	}
}
