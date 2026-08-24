package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStoreSkill creates a valid Skill directory under root and returns
// its path.
func writeStoreSkill(t *testing.T, root, slug string) string {
	t.Helper()
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := "---\nname: " + slug + "\ndescription: test\n---\n# " + slug + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustIsolation(t *testing.T) string {
	t.Helper()
	name, err := NewIsolationName()
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func TestInspectMatrix(t *testing.T) {
	base := physicalTemp(t)
	store := filepath.Join(base, "store")
	container := filepath.Join(base, "skills")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStoreSkill(t, store, "demo")
	writeStoreSkill(t, store, "broken-target")

	// missing entry
	entries, err := Inspect(container, []Relation{{Slug: "demo"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedMissing {
		t.Fatalf("missing: %+v", entries[0])
	}

	// managed linked: symlink matching its recorded identity and raw target
	raw := filepath.Join(store, "demo")
	if err := os.Symlink(raw, filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	_, dev, ino, mtime, err := ProbeSymlink(container, "demo")
	if err != nil {
		t.Fatal(err)
	}
	owned := Relation{Slug: "demo", LedgerRaw: raw, LedgerDev: dev, LedgerIno: ino, LedgerMtime: mtime}
	entries, err = Inspect(container, []Relation{owned}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedLinked || !entries[0].Managed || entries[0].Adoptable {
		t.Fatalf("linked: %+v", entries[0])
	}

	// unmanaged symlink to the exact Store Skill: conflict, adoptable
	entries, err = Inspect(container, []Relation{{Slug: "demo"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedConflict || entries[0].Managed || !entries[0].Adoptable {
		t.Fatalf("adoptable conflict: %+v", entries[0])
	}

	// managed symlink whose destination is gone: broken_link
	if err := os.RemoveAll(filepath.Join(store, "demo")); err != nil {
		t.Fatal(err)
	}
	entries, err = Inspect(container, []Relation{owned}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedBrokenLink || !entries[0].Managed {
		t.Fatalf("broken_link: %+v", entries[0])
	}

	// unmanaged symlink outside the Store: conflict, never adoptable
	writeStoreSkill(t, base, "outside")
	other := filepath.Join(base, "outside")
	if err := os.Symlink(other, filepath.Join(container, "broken-target")); err != nil {
		t.Fatal(err)
	}
	entries, err = Inspect(container, []Relation{{Slug: "broken-target"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedConflict || entries[0].Adoptable {
		t.Fatalf("outside conflict: %+v", entries[0])
	}

	// a regular file is a conflict with its node kind reported
	file := filepath.Join(container, "plain")
	if err := os.WriteFile(file, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err = Inspect(container, []Relation{{Slug: "plain"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedConflict || entries[0].NodeKind != KindFile {
		t.Fatalf("file conflict: %+v", entries[0])
	}
}

// TestInspectSameRawReplacementIsNotManaged proves leftover ledger state
// is not ownership: a new symlink with the same raw target, or a migrated
// row with zero identity, stays unmanaged.
func TestInspectSameRawReplacementIsNotManaged(t *testing.T) {
	base := physicalTemp(t)
	store := filepath.Join(base, "store")
	container := filepath.Join(base, "skills")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStoreSkill(t, store, "demo")
	raw := filepath.Join(store, "demo")
	link := filepath.Join(container, "demo")
	if err := os.Symlink(raw, link); err != nil {
		t.Fatal(err)
	}
	_, dev, ino, mtime, err := ProbeSymlink(container, "demo")
	if err != nil {
		t.Fatal(err)
	}
	rel := Relation{Slug: "demo", LedgerRaw: raw, LedgerDev: dev, LedgerIno: ino, LedgerMtime: mtime}
	entries, err := Inspect(container, []Relation{rel}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedLinked || !entries[0].Managed {
		t.Fatalf("original: %+v", entries[0])
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(raw, link); err != nil {
		t.Fatal(err)
	}
	_, dev2, ino2, mtime2, err := ProbeSymlink(container, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if dev2 == rel.LedgerDev && ino2 == rel.LedgerIno && mtime2 == rel.LedgerMtime {
		t.Fatal("replacement reused the recorded identity")
	}
	entries, err = Inspect(container, []Relation{rel}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Observed != ObservedConflict || entries[0].Managed || !entries[0].Adoptable {
		t.Fatalf("same-raw replacement: %+v", entries[0])
	}

	entries, err = Inspect(container, []Relation{{Slug: "demo", LedgerRaw: raw}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Managed || entries[0].Observed != ObservedConflict {
		t.Fatalf("zero identity: %+v", entries[0])
	}
}

func TestCreateLinkNoOverwrite(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")

	// the missing container is created
	if err := CreateLink(container, "demo", raw); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(filepath.Join(container, "demo"))
	if err != nil || got != raw {
		t.Fatalf("readlink: %q, %v", got, err)
	}

	// an existing entry is never overwritten
	before := []byte("mine")
	if err := os.WriteFile(filepath.Join(container, "other"), before, 0o644); err != nil {
		t.Fatal(err)
	}
	err = CreateLink(container, "other", raw)
	if err != ErrEntryExists {
		t.Fatalf("existing entry: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(container, "other"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("entry changed: %q, %v", after, err)
	}

	// an existing symlink is also never overwritten
	if err := os.Symlink("elsewhere", filepath.Join(container, "link")); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(container, "link", raw); err != ErrEntryExists {
		t.Fatalf("existing symlink: %v", err)
	}
	if got, _ := os.Readlink(filepath.Join(container, "link")); got != "elsewhere" {
		t.Fatalf("symlink changed: %q", got)
	}
}

func TestRemoveManagedLink(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := CreateLink(container, "demo", raw); err != nil {
		t.Fatal(err)
	}

	// a matching symlink is removed
	rr, err := RemoveManagedLink(container, "demo", raw, mustIsolation(t))
	if err != nil || rr != RemoveDone {
		t.Fatalf("remove: %v, %v", rr, err)
	}
	if _, err := os.Lstat(filepath.Join(container, "demo")); !os.IsNotExist(err) {
		t.Fatalf("link still present: %v", err)
	}

	// a missing entry reports absent
	rr, err = RemoveManagedLink(container, "demo", raw, mustIsolation(t))
	if err != nil || rr != RemoveAbsent {
		t.Fatalf("absent: %v, %v", rr, err)
	}

	// a foreign entry is never removed
	if err := os.WriteFile(filepath.Join(container, "demo"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr, err = RemoveManagedLink(container, "demo", raw, mustIsolation(t))
	if err != nil || rr != RemoveMismatch {
		t.Fatalf("mismatch: %v, %v", rr, err)
	}
	if data, _ := os.ReadFile(filepath.Join(container, "demo")); string(data) != "mine" {
		t.Fatal("foreign entry was removed")
	}

	// a different symlink is never removed
	if err := os.Remove(filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("elsewhere", filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	rr, err = RemoveManagedLink(container, "demo", raw, mustIsolation(t))
	if err != nil || rr != RemoveMismatch {
		t.Fatalf("different symlink: %v, %v", rr, err)
	}
	if got, _ := os.Readlink(filepath.Join(container, "demo")); got != "elsewhere" {
		t.Fatal("symlink was removed")
	}
}

func TestProbeAdoption(t *testing.T) {
	base := physicalTemp(t)
	store := filepath.Join(base, "store")
	container := filepath.Join(base, "skills")
	writeStoreSkill(t, store, "demo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	expected := ExpectedPath(store, "demo")
	raw := expected
	if err := os.Symlink(raw, filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	got, err := ProbeAdoption(container, "demo", expected)
	if err != nil || got != raw {
		t.Fatalf("adopt: %q, %v", got, err)
	}

	// a file cannot be adopted
	if err := os.WriteFile(filepath.Join(container, "plain"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeAdoption(container, "plain", expected); err == nil {
		t.Fatal("file adoption must fail")
	}
	// a wrong-target link cannot be adopted
	if err := os.Symlink(filepath.Join(base, "elsewhere"), filepath.Join(container, "wrong")); err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeAdoption(container, "wrong", expected); err == nil {
		t.Fatal("wrong-target adoption must fail")
	}
}
