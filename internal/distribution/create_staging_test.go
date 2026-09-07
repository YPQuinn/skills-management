package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateRecordsIdentityBeforePublication(t *testing.T) {
	base := physicalTemp(t)
	container, raw := filepath.Join(base, "target"), filepath.Join(base, "store", "demo")
	slot := mustIsolation(t)
	var recorded LinkProof
	proof, err := CreateLink(container, "demo", raw, slot, func(p LinkProof) error {
		if _, err := os.Lstat(filepath.Join(container, "demo")); !os.IsNotExist(err) {
			t.Fatalf("published before identity persistence: %v", err)
		}
		got, err := ProbeSymlink(filepath.Join(container, slot), IsolatedEntry)
		if err != nil || !p.Matches(got) {
			t.Fatalf("staged proof: %+v, %v", got, err)
		}
		recorded = p
		return nil
	})
	if err != nil || !recorded.Matches(proof) {
		t.Fatalf("publication: %+v, %v", proof, err)
	}
	if err := VerifyLink(container, "demo", recorded); err != nil {
		t.Fatal(err)
	}
	if err := DiscardCreateStaging(container, slot, recorded); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(container)
	if err != nil || len(entries) != 1 || entries[0].Name() != "demo" {
		t.Fatalf("staging leaked: %+v, %v", entries, err)
	}
}

func TestCreatePersistenceFailureDoesNotPublish(t *testing.T) {
	base := physicalTemp(t)
	container, raw := filepath.Join(base, "target"), filepath.Join(base, "store", "demo")
	slot := mustIsolation(t)
	failed := errors.New("database write failed")
	proof, err := CreateLink(container, "demo", raw, slot, func(LinkProof) error { return failed })
	if !errors.Is(err, failed) || !proof.Proven() {
		t.Fatalf("persistence failure: %+v, %v", proof, err)
	}
	if _, err := os.Lstat(filepath.Join(container, "demo")); !os.IsNotExist(err) {
		t.Fatalf("published after failed persistence: %v", err)
	}
	if err := DiscardCreateStaging(container, slot, proof); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(container)
	if len(entries) != 0 {
		t.Fatalf("failed create left staging: %+v", entries)
	}
}

func TestCreateDoesNotOverwriteEntryAppearingBeforePublish(t *testing.T) {
	base := physicalTemp(t)
	container, raw := filepath.Join(base, "target"), filepath.Join(base, "store", "demo")
	slot := mustIsolation(t)
	link := filepath.Join(container, "demo")
	proof, err := CreateLink(container, "demo", raw, slot, func(LinkProof) error {
		return os.Symlink(raw, link)
	})
	if !errors.Is(err, ErrEntryExists) {
		t.Fatalf("concurrent create: %v", err)
	}
	if err := DiscardCreateStaging(container, slot, proof); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != raw {
		t.Fatalf("external link changed: %q, %v", got, err)
	}
	if err := VerifyLink(container, "demo", proof); !errors.Is(err, ErrLinkMismatch) {
		t.Fatalf("external entry received staging ownership: %v", err)
	}
}

func TestCreateCleanupPreservesReplacedStaging(t *testing.T) {
	base := physicalTemp(t)
	container, raw := filepath.Join(base, "target"), filepath.Join(base, "store", "demo")
	slot := mustIsolation(t)
	staged := filepath.Join(container, slot, IsolatedEntry)
	proof, err := CreateLink(container, "demo", raw, slot, func(LinkProof) error {
		replaceSameRaw(t, staged, raw)
		return nil
	})
	if !errors.Is(err, ErrLinkMismatch) {
		t.Fatalf("replaced staging: %v", err)
	}
	for _, p := range []LinkProof{proof, {}} {
		if err := DiscardCreateStaging(container, slot, p); !errors.Is(err, ErrLinkMismatch) {
			t.Fatalf("cleanup must reject mismatched or unproven staging: %v", err)
		}
		if got, err := os.Readlink(staged); err != nil || got != raw {
			t.Fatalf("staging replacement removed: %q, %v", got, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(container, "demo")); !os.IsNotExist(err) {
		t.Fatalf("staging replacement published: %v", err)
	}
}

func TestCreateOccupiedStagingIsUntouched(t *testing.T) {
	container := physicalTemp(t)
	slot := mustIsolation(t)
	if err := os.Mkdir(filepath.Join(container, slot), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := CreateLink(container, "demo", "/store/demo", slot, func(LinkProof) error {
		t.Fatal("persist called for occupied staging")
		return nil
	})
	if !errors.Is(err, ErrSlotOccupied) {
		t.Fatalf("occupied directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(container, slot)); err != nil {
		t.Fatal("occupied directory removed")
	}
}
