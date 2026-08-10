package state

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"skillctl/internal/source"
)

func testSource(name string) source.Source {
	now := time.Now().UTC()
	return source.Source{
		Name:                  name,
		Locator:               source.Locator{Kind: source.KindLocal, Location: "/tmp/" + name},
		CreatedAt:             now,
		UpdatedAt:             now,
		Available:             true,
		LastCheckStartedAt:    &now,
		LastCheckedAt:         &now,
		LastCheckResult:       source.CheckResultOK,
		LastSuccessfulCheckAt: &now,
		LastCommit:            "abc123",
		LastInventoryDigest:   "digest-of-inventory",
		Entries: []source.Entry{
			{RelativeDir: "skills/a", Name: "A", Description: "one", Digest: "digest-of-a"},
		},
	}
}

func TestInsertAndGetSource(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := testSource("alpha")
	id, err := InsertSource(db, s)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" || got.Kind != source.KindLocal || got.Location != "/tmp/alpha" {
		t.Fatalf("source: %+v", got)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "A" {
		t.Fatalf("inventory: %+v", got.Entries)
	}
	if got.Entries[0].Digest != "digest-of-a" {
		t.Fatalf("entry digest: got %q", got.Entries[0].Digest)
	}
	if got.LastCheckResult != source.CheckResultOK || got.LastInventoryDigest != "digest-of-inventory" {
		t.Fatalf("check metadata: %+v", got)
	}
	if got.LastCheckStartedAt == nil || got.LastCheckedAt == nil || got.LastSuccessfulCheckAt == nil {
		t.Fatalf("observation times: %+v", got)
	}

	if _, err := GetSource(db, id+100); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing source: got %v", err)
	}
}

func TestInsertSourceUniqueTuple(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := InsertSource(db, testSource("a")); err != nil {
		t.Fatal(err)
	}
	dup := testSource("a")
	dup.Location = "/tmp/a" // same tuple, different name
	if _, err := InsertSource(db, dup); err == nil || !IsUniqueViolation(err) {
		t.Fatalf("duplicate tuple: got %v, want a unique violation", err)
	}
	// the same location with a different subpath is a different Source
	other := testSource("a-sub")
	other.Location = "/tmp/a"
	other.Subpath = "skills"
	if _, err := InsertSource(db, other); err != nil {
		t.Fatalf("different subpath: %v", err)
	}
}

func TestReplaceSourceObservationWholeInventory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := InsertSource(db, testSource("alpha"))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	updated := source.Source{
		UpdatedAt: now, Available: true,
		LastCheckStartedAt: &now, LastCheckedAt: &now,
		LastCheckResult:       source.CheckResultOK,
		LastSuccessfulCheckAt: &now,
		LastCommit:            "abc123",
		LastInventoryDigest:   "digest-v2",
		Entries: []source.Entry{
			{RelativeDir: "skills/x", Name: "X", Description: "new", Digest: "digest-x"},
			{RelativeDir: "skills/y", Name: "Y", Description: "also new", Digest: "digest-y"},
		},
		Issues: []source.Issue{{RelativeDir: "skills/bad", Reason: "missing name"}},
	}
	if err := ReplaceSourceObservation(db, id, updated); err != nil {
		t.Fatal(err)
	}
	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || len(got.Issues) != 1 || got.LastCommit != "abc123" {
		t.Fatalf("replaced observation: %+v", got)
	}
	if got.LastInventoryDigest != "digest-v2" || got.LastCheckResult != source.CheckResultOK {
		t.Fatalf("replaced metadata: %+v", got)
	}
	if got.Entries[0].Digest != "digest-x" || got.Entries[1].Digest != "digest-y" {
		t.Fatalf("replaced entry digests: %+v", got.Entries)
	}
}

func TestUpdateSourceStatusKeepsInventory(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	id, err := InsertSource(db, testSource("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	failed := source.Source{
		UpdatedAt: now, Available: false, LastError: "checking: git: boom",
		LastCheckStartedAt: &now, LastCheckedAt: &now,
		LastCheckResult: source.CheckResultFailed,
	}
	if err := UpdateSourceStatus(db, id, failed); err != nil {
		t.Fatal(err)
	}
	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Available || got.LastError == "" || got.LastSuccessfulCheckAt == nil {
		t.Fatalf("status: %+v", got)
	}
	if got.LastCheckResult != source.CheckResultFailed {
		t.Fatalf("failed result: got %q", got.LastCheckResult)
	}
	// a failed check retains the previous commit, aggregate digest, and
	// per-entry digest
	if got.LastCommit != "abc123" || got.LastInventoryDigest != "digest-of-inventory" {
		t.Fatalf("failed check must retain commit and digest: %+v", got)
	}
	if len(got.Entries) != 1 || got.Entries[0].Digest != "digest-of-a" {
		t.Fatalf("inventory must survive a failed check: %+v", got.Entries)
	}
}

func TestListSourcesAndResolveByName(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := InsertSource(db, testSource("beta")); err != nil {
		t.Fatal(err)
	}
	if _, err := InsertSource(db, testSource("alpha")); err != nil {
		t.Fatal(err)
	}
	list, err := ListSources(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list: %d sources", len(list))
	}
	// ordered by name: alpha, beta
	if list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("order: %+v", list)
	}
	if list[0].EntryCount != 1 || list[0].Stale {
		t.Fatalf("summary: %+v", list[0])
	}

	id, err := SourceIDByName(db, "alpha")
	if err != nil || id != 2 {
		t.Fatalf("by name: %d, %v", id, err)
	}
	if _, err := SourceIDByName(db, "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown name: %v", err)
	}
	exists, err := SourceNameExists(db, "alpha")
	if err != nil || !exists {
		t.Fatalf("existing name: %v, %v", exists, err)
	}
	if exists, err := SourceNameExists(db, "nope"); err != nil || exists {
		t.Fatalf("unknown name exists: %v, %v", exists, err)
	}

	// a second Source may not reuse a name; the unique index refuses it
	dup := testSource("alpha")
	dup.Location = "/tmp/other"
	if _, err := InsertSource(db, dup); err == nil || !IsUniqueViolation(err) {
		t.Fatalf("duplicate name: got %v, want a unique violation", err)
	}
}

func TestSourcePersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := InsertSource(db, testSource("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := GetSource(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" || len(got.Entries) != 1 {
		t.Fatalf("after reopen: %+v", got)
	}
}
