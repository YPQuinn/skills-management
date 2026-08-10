package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// cancelOnCheck is a context whose Err method cancels the underlying
// context once it has been consulted limit times. It makes mid-loop
// cancellation deterministic: the loops that honor cancellation check
// ctx.Err() between chunks or nodes.
type cancelOnCheck struct {
	context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	n      int
	limit  int
}

func (c *cancelOnCheck) Err() error {
	c.mu.Lock()
	c.n++
	limit := c.limit
	c.mu.Unlock()
	if c.n >= limit {
		c.cancel()
	}
	return c.Context.Err()
}

func TestDiscoverCtxCancelled(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "a"), "A", "one")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DiscoverCtx(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled discovery: %v", err)
	}
}

func TestSnapshotRootCtxCancelled(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "a"), "A", "one")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := snapshotRoot(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled snapshot: %v", err)
	}
}

// TestSnapshotTreeCancelledMidWalk proves the snapshot walk aborts promptly
// when cancellation happens between visited nodes.
func TestSnapshotTreeCancelledMidWalk(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "alpha")
	writeSkill(t, skillDir, "Alpha", "one")
	if err := os.WriteFile(filepath.Join(skillDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnCheck{Context: base, cancel: cancel, limit: 2}
	var snap snapshot
	if err := snapshotTree(ctx, skillDir, "skills/alpha", &snap); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-walk cancellation: %v", err)
	}
}

func TestHashFileCtxCancelledBeforeRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.bin")
	if err := os.WriteFile(path, make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hashFileCtx(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled hash: %v", err)
	}
}

// TestHashFileCtxCancelledMidHash proves hashing a large file aborts
// promptly on cancellation: the file is read in 1 MiB chunks and the
// context is checked between chunks, so the hash cannot run to completion
// after cancellation.
func TestHashFileCtxCancelledMidHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bin")
	if err := os.WriteFile(path, make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnCheck{Context: base, cancel: cancel, limit: 2}
	if _, err := hashFileCtx(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-hash cancellation: %v", err)
	}
}

func TestLocalObserveCancelled(t *testing.T) {
	root := realTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "a"), "A", "one")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Local{stabilityInterval: time.Millisecond}).Observe(
		ctx, Locator{Kind: KindLocal, Location: root}, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled local observe: %v", err)
	}
}
