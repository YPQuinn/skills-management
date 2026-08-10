package lock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	first, err := TryExclusive(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := TryExclusive(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second exclusive lock: got %v, want ErrLocked", err)
	}
	if _, err := TryShared(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("shared lock under an exclusive lock: got %v, want ErrLocked", err)
	}

	if err := first.Unlock(); err != nil {
		t.Fatal(err)
	}
	second, err := TryExclusive(path)
	if err != nil {
		t.Fatalf("exclusive lock after unlock: %v", err)
	}
	if err := second.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedLocksCoexist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	a, err := TryShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Unlock()

	b, err := TryShared(path)
	if err != nil {
		t.Fatalf("second shared lock: %v", err)
	}
	if _, err := TryExclusive(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("exclusive lock under shared locks: got %v, want ErrLocked", err)
	}

	if err := b.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err := a.Unlock(); err != nil {
		t.Fatal(err)
	}
	ex, err := TryExclusive(path)
	if err != nil {
		t.Fatalf("exclusive lock after releasing shared locks: %v", err)
	}
	ex.Unlock()
}

func TestLockFileCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := TryExclusive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Unlock()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file was not created: %v", err)
	}
}
