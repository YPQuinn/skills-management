package distribution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDoesNotDeleteGuessableTemp(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	guessable := filepath.Join(container, ".skillctl-create-demo")
	if err := os.Symlink(raw, guessable); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(container, "demo", raw); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(guessable); err != nil || got != raw {
		t.Fatalf("guessable temp was deleted: %q, %v", got, err)
	}
}

func TestCreateDoesNotOverwriteUserMatchingSymlink(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(raw, filepath.Join(container, "demo")); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(container, "demo", raw); err != ErrEntryExists {
		t.Fatalf("user symlink: %v", err)
	}
	if got, _ := os.Readlink(filepath.Join(container, "demo")); got != raw {
		t.Fatalf("user symlink changed: %q", got)
	}
}

func TestCreateRefusesFinalPathReplacement(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, "demo"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CreateLink(container, "demo", raw); err != ErrEntryExists {
		t.Fatalf("replaced final path: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(container, "demo")); err != nil || string(data) != "user" {
		t.Fatalf("final path overwritten: %q, %v", data, err)
	}
}

func TestRemoveDoesNotDeleteGuessableName(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := CreateLink(container, "demo", raw); err != nil {
		t.Fatal(err)
	}
	guessable := filepath.Join(container, ".skillctl-remove-demo")
	if err := os.Symlink(raw, guessable); err != nil {
		t.Fatal(err)
	}
	if rr, err := RemoveManagedLink(container, "demo", raw, mustIsolation(t)); err != nil || rr != RemoveDone {
		t.Fatalf("remove: %v, %v", rr, err)
	}
	if got, err := os.Readlink(guessable); err != nil || got != raw {
		t.Fatalf("guessable name was deleted: %q, %v", got, err)
	}
}

func TestRemoveOccupiedIsolationDir(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := CreateLink(container, "demo", raw); err != nil {
		t.Fatal(err)
	}
	iso := mustIsolation(t)
	if err := os.WriteFile(filepath.Join(container, iso), []byte("preoccupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr, err := RemoveManagedLink(container, "demo", raw, iso)
	if err == nil || rr != RemoveAbsent {
		t.Fatalf("occupied isolation: %v, %v", rr, err)
	}
	if got, _ := os.Readlink(filepath.Join(container, "demo")); got != raw {
		t.Fatal("managed link was removed")
	}
	if data, err := os.ReadFile(filepath.Join(container, iso)); err != nil || string(data) != "preoccupied" {
		t.Fatalf("preoccupied isolation deleted: %q, %v", data, err)
	}
}

func TestRemoveRestoresReplacedFinalPath(t *testing.T) {
	base := physicalTemp(t)
	container := filepath.Join(base, "skills")
	raw := filepath.Join(base, "store", "demo")
	if err := os.MkdirAll(container, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(container, "demo"), []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	rr, err := RemoveManagedLink(container, "demo", raw, mustIsolation(t))
	if err != nil || rr != RemoveMismatch {
		t.Fatalf("replaced final path: %v, %v", rr, err)
	}
	if data, err := os.ReadFile(filepath.Join(container, "demo")); err != nil || string(data) != "user" {
		t.Fatalf("replacement lost: %q, %v", data, err)
	}
}
