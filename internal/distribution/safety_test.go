package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveOccupiedIsolationSlot(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	proof := mustCreateLink(t, container, "demo", raw)
	slotName := mustIsolation(t)
	slot := filepath.Join(container, slotName)
	if err := os.WriteFile(slot, []byte("foreign-slot"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr, err := RemoveManagedLink(container, "demo", proof, slotName)
	if err == nil || rr != RemoveAbsent {
		t.Fatalf("occupied slot must fail closed: %v, %v", rr, err)
	}
	if got, _ := os.Readlink(filepath.Join(container, "demo")); got != raw {
		t.Fatalf("managed link changed: %q", got)
	}
	if data, err := os.ReadFile(slot); err != nil || string(data) != "foreign-slot" {
		t.Fatalf("isolation slot overwritten: %q, %v", data, err)
	}
}

func TestRemoveRestoreDoesNotOverwrite(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(base, "store", "demo")
	slotName := mustIsolation(t)
	slot := filepath.Join(container, slotName)
	if err := os.WriteFile(slot, []byte("isolated-foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, "demo"), []byte("live-foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr, err := RemoveManagedLink(container, "demo", dummyProof(raw), slotName)
	if err != nil || rr != RemoveMismatch {
		t.Fatalf("foreign live entry: %v, %v", rr, err)
	}
	if data, err := os.ReadFile(filepath.Join(container, "demo")); err != nil || string(data) != "live-foreign" {
		t.Fatalf("live entry overwritten: %q, %v", data, err)
	}
	if data, err := os.ReadFile(slot); err != nil || string(data) != "isolated-foreign" {
		t.Fatalf("slot overwritten: %q, %v", data, err)
	}
}

func TestFinishIsolatedRemoveRestoreDoesNotOverwrite(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := filepath.Join(base, "store", "demo")
	iso := mustIsolation(t)
	if err := CreateIsolationDir(container, iso); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, iso, IsolatedEntry), []byte("isolated-foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, "demo"), []byte("live-foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	rr, err := FinishIsolatedRemove(container, "demo", dummyProof(raw), iso)
	if err == nil || rr != RemoveAbsent {
		t.Fatalf("restore must not overwrite: %v, %v", rr, err)
	}
	if data, err := os.ReadFile(filepath.Join(container, "demo")); err != nil || string(data) != "live-foreign" {
		t.Fatalf("live entry overwritten: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(container, iso, IsolatedEntry)); err != nil || string(data) != "isolated-foreign" {
		t.Fatalf("isolated entry deleted: %q, %v", data, err)
	}
}

func TestPinnedHandleRejectsPrefixSymlink(t *testing.T) {
	base := physicalTemp(t)
	store := filepath.Join(base, "store")
	container := filepath.Join(base, "skills")
	elsewhere := filepath.Join(base, "elsewhere")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStoreSkill(t, store, "demo")
	raw := filepath.Join(store, "demo")
	if err := os.Symlink(raw, filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(container); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, container); err != nil {
		t.Fatal(err)
	}

	if _, err := Inspect(container, []Relation{{Slug: "demo", LedgerRaw: raw}}, store); err == nil {
		t.Fatal("inspect must refuse a prefix symlink")
	}
	if _, err := createTestLink(t, container, "other", raw); err == nil {
		t.Fatal("create must refuse a prefix symlink")
	}
	if _, err := RemoveManagedLink(container, "demo", dummyProof(raw), mustIsolation(t)); err == nil {
		t.Fatal("remove must refuse a prefix symlink")
	}
	if _, err := ProbeAdoption(container, "demo", raw); err == nil {
		t.Fatal("adoption must refuse a prefix symlink")
	}
	if _, err := os.Lstat(filepath.Join(elsewhere, "other")); !os.IsNotExist(err) {
		t.Fatal("the redirected location must stay empty")
	}
}
