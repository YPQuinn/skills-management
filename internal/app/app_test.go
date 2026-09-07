package app

import (
	"path/filepath"
	"testing"
)

func TestNewAndClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a, err := New(filepath.Join(dir, "store"), filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if a.StorePath != filepath.Join(dir, "store") {
		t.Fatalf("StorePath: got %q", a.StorePath)
	}
	if a.StateDBPath != filepath.Join(dir, "state.db") {
		t.Fatalf("StateDBPath: got %q", a.StateDBPath)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}
