package state

import (
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func v12FS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := v11FS(t)
	data, err := migrationsFS.ReadFile("migrations/012_managed_link_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	(*fsys)["migrations/012_managed_link_identity.sql"] = &fstest.MapFile{Data: data}
	return fsys
}

// TestMigration013PreservesOpenCreateIntents proves a version-12 database
// with an unfinished create intent upgrades by adding identity columns
// defaulting to 0, so recovery cannot claim the row.
func TestMigration013PreservesOpenCreateIntents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v12FS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 12 {
		t.Fatalf("v12 schema: got %d, want 12", got)
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
		(target_id, skill_id, action, link_path, raw_target, slot_name, phase, created_at)
		VALUES (?, ?, 'create', ?, ?, '', 'planned', ?)`,
		targetID, skillID, "/tmp/t/alpha", "/store/alpha", timeToSQL(&now)); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := appliedVersion(t, db); got != 13 {
		t.Fatalf("migrated schema: got %d, want 13", got)
	}
	intents, err := ListOpenLinkIntents(db)
	if err != nil || len(intents) != 1 {
		t.Fatalf("legacy intent: %+v, %v", intents, err)
	}
	if intents[0].LinkDev != 0 || intents[0].LinkIno != 0 || intents[0].LinkMtime != 0 {
		t.Fatalf("unproven identity: %+v", intents[0])
	}
}
