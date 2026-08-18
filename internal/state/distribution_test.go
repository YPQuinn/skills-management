package state

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// v8FS returns a migration set containing only 001-008, the schema the
// previous build left on disk.
func v8FS(t *testing.T) *fstest.MapFS {
	t.Helper()
	fsys := &fstest.MapFS{}
	for _, name := range []string{
		"001_initial.sql", "002_sources.sql", "003_source_observation.sql",
		"004_unique_source_name.sql", "005_skills.sql", "006_terminal_receipts.sql",
		"007_groups_targets.sql", "008_sync.sql",
	} {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		(*fsys)["migrations/"+name] = &fstest.MapFile{Data: data}
	}
	return fsys
}

// TestMigration009Distribution is the upgrade path: a version-8 database
// holds Skills and Targets; opening it applies migration 009, which adds
// the Managed Link ledger, durable link intents, Distribution Status rows,
// and the per-Target inspection and outcome columns while preserving every
// existing row.
func TestMigration009Distribution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, v8FS(t)); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersion(t, db); got != 8 {
		t.Fatalf("legacy schema: got %d, want 8", got)
	}
	now := time.Now().UTC()
	targetID, err := InsertTarget(db, Target{
		Name: "custom-1", Path: "/tmp/target", Adapter: "custom", Scope: "custom",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(testSourceID(t, db), 0))
	if err != nil {
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
	// The upgraded schema accepts the new entities and keeps the old rows.
	target, err := GetTargetByID(db, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "custom-1" || target.LastInspectedAt != nil {
		t.Fatalf("preserved target: %+v", target)
	}
	if _, err := InsertManagedLink(db, ManagedLink{
		TargetID: targetID, SkillID: skillID, LinkPath: "/tmp/target/alpha",
		RawTarget: "/store/alpha", EstablishedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertLinkIntent(db, LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "create",
		LinkPath: "/tmp/target/alpha", RawTarget: "/store/alpha", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceDistributionItems(db, targetID, []DistributionItem{{
		TargetID: targetID, SkillID: skillID, Desired: "present", Observed: "linked",
		Managed: true, InspectedAt: &now,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTargetInspection(db, targetID, TargetDistribution{InspectedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTargetOutcome(db, targetID, "succeeded", now, now, ""); err != nil {
		t.Fatal(err)
	}
	target, err = GetTargetByID(db, targetID)
	if err != nil {
		t.Fatal(err)
	}
	if target.LastDistResult != "succeeded" || target.LastInspectedAt == nil {
		t.Fatalf("outcome metadata: %+v", target)
	}
}

func TestManagedLinkLedger(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(testSourceID(t, db), 0))
	if err != nil {
		t.Fatal(err)
	}
	targetID, err := InsertTarget(db, Target{
		Name: "t1", Path: "/tmp/t", Adapter: "custom", Scope: "custom",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	link := ManagedLink{TargetID: targetID, SkillID: skillID, LinkPath: "/tmp/t/alpha", RawTarget: "/store/alpha", EstablishedAt: now}
	if _, err := InsertManagedLink(db, link); err != nil {
		t.Fatal(err)
	}
	got, err := GetManagedLink(db, targetID, skillID)
	if err != nil || got.RawTarget != "/store/alpha" {
		t.Fatalf("get: %+v, %v", got, err)
	}
	// a duplicate claim is rejected by the unique index
	if _, err := InsertManagedLink(db, link); !IsUniqueViolation(err) {
		t.Fatalf("duplicate: %v", err)
	}
	// replacement re-records the raw target (adoption)
	link.RawTarget = "/store/alpha-new"
	if err := ReplaceManagedLink(db, link); err != nil {
		t.Fatal(err)
	}
	got, err = GetManagedLink(db, targetID, skillID)
	if err != nil || got.RawTarget != "/store/alpha-new" {
		t.Fatalf("replaced: %+v, %v", got, err)
	}

	list, err := ListManagedLinksByTarget(db, targetID)
	if err != nil || len(list) != 1 || list[0].Slug != "alpha" {
		t.Fatalf("list: %+v, %v", list, err)
	}
	if err := DeleteManagedLink(db, targetID, skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := GetManagedLink(db, targetID, skillID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted claim still present: %v", err)
	}
}

func TestLinkIntentFinalize(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(testSourceID(t, db), 0))
	if err != nil {
		t.Fatal(err)
	}
	targetID, err := InsertTarget(db, Target{
		Name: "t1", Path: "/tmp/t", Adapter: "custom", Scope: "custom",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	intentID, err := InsertLinkIntent(db, LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "create",
		LinkPath: "/tmp/t/alpha", RawTarget: "/store/alpha",
		SlotName: ".skillctl-c-abcd", Phase: LinkPhasePlanned, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := SetLinkIntentPhase(db, intentID, LinkPhasePrepared); err != nil {
		t.Fatal(err)
	}
	got, err := ListOpenLinkIntents(db)
	if err != nil || len(got) != 1 || got[0].SlotName != ".skillctl-c-abcd" || got[0].Phase != LinkPhasePrepared {
		t.Fatalf("progress: %+v, %v", got, err)
	}
	if err := FinalizeCreateLedger(db, ManagedLink{
		TargetID: targetID, SkillID: skillID, LinkPath: "/tmp/t/alpha",
		RawTarget: "/store/alpha", EstablishedAt: now,
	}, intentID); err != nil {
		t.Fatal(err)
	}
	if got, _ := ListOpenLinkIntents(db); len(got) != 0 {
		t.Fatalf("intent not cleared: %+v", got)
	}
	if _, err := GetManagedLink(db, targetID, skillID); err != nil {
		t.Fatalf("ledger missing: %v", err)
	}

	intentID, err = InsertLinkIntent(db, LinkIntent{
		TargetID: targetID, SkillID: skillID, Action: "remove",
		LinkPath: "/tmp/t/alpha", RawTarget: "/store/alpha", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := FinalizeRemoveLedger(db, targetID, skillID, intentID); err != nil {
		t.Fatal(err)
	}
	if got, _ := ListOpenLinkIntents(db); len(got) != 0 {
		t.Fatalf("remove intent not cleared: %+v", got)
	}
	if _, err := GetManagedLink(db, targetID, skillID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ledger still present: %v", err)
	}
}

func TestDistributionItemsAndOutcomes(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	skillID, err := InsertSkillAndBinding(db, testSkill("alpha"), testBinding(testSourceID(t, db), 0))
	if err != nil {
		t.Fatal(err)
	}
	targetID, err := InsertTarget(db, Target{
		Name: "t1", Path: "/tmp/t", Adapter: "custom", Scope: "custom",
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	// a complete observation replaces wholesale
	items := []DistributionItem{{
		TargetID: targetID, SkillID: skillID, Desired: "present", Observed: "conflict",
		Adoptable: true, NodeKind: "symlink", RawTarget: "/store/alpha", InspectedAt: &now,
	}}
	if err := ReplaceDistributionItems(db, targetID, items); err != nil {
		t.Fatal(err)
	}
	list, err := ListDistributionItems(db, targetID)
	if err != nil || len(list) != 1 || list[0].Slug != "alpha" || !list[0].Adoptable {
		t.Fatalf("list: %+v, %v", list, err)
	}

	// an outcome updates the existing row
	if err := UpdateDistributionItemOutcome(db, targetID, skillID, "present", "linked", "created", "", now); err != nil {
		t.Fatal(err)
	}
	list, err = ListDistributionItems(db, targetID)
	if err != nil || list[0].Observed != "linked" || list[0].LastResult != "created" {
		t.Fatalf("outcome: %+v, %v", list, err)
	}

	// marking stale retains the observation and flags it
	if err := MarkDistributionItemsStale(db, targetID, "boom"); err != nil {
		t.Fatal(err)
	}
	list, err = ListDistributionItems(db, targetID)
	if err != nil || !list[0].Stale || list[0].LastError != "boom" {
		t.Fatalf("stale: %+v, %v", list, err)
	}

	// a later complete observation removes relations it no longer covers
	if err := ReplaceDistributionItems(db, targetID, nil); err != nil {
		t.Fatal(err)
	}
	if list, _ := ListDistributionItems(db, targetID); len(list) != 0 {
		t.Fatalf("stale rows survived: %+v", list)
	}
}
