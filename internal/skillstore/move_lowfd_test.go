//go:build darwin || linux

package skillstore

import (
	"context"
	"os"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"testing"

	"skillctl/internal/source"
)

// TestMovePinnedDoesNotLeakDestinationHandles proves movePinned closes its
// destination root on the success path. With the soft open-file limit
// lowered and the GC disabled — so a leaked *os.Root is never recycled by
// a finalizer mid-test — hundreds of proven moves must complete without
// EMFILE. Before the fix every successful move leaked the destination
// handle and the loop failed around the rlimit; after the fix the peak
// concurrent FDs stay far below the limit. The original rlimit and GC
// setting are restored afterwards; the test never runs in parallel.
func TestMovePinnedDoesNotLeakDestinationHandles(t *testing.T) {
	var orig syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &orig); err != nil {
		t.Fatal(err)
	}
	lim := orig
	if lim.Cur > 128 {
		lim.Cur = 128
		if lim.Max < lim.Cur {
			lim.Cur = lim.Max
		}
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if rerr := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &orig); rerr != nil {
				t.Errorf("restoring RLIMIT_NOFILE: %v", rerr)
			}
		})
	}
	gcPercent := debug.SetGCPercent(-1)
	t.Cleanup(func() { debug.SetGCPercent(gcPercent) })

	s := newStore(t)
	if err := s.EnsureLayout(); err != nil {
		t.Fatal(err)
	}
	layout, err := s.openLayout()
	if err != nil {
		t.Fatal(err)
	}
	defer layout.close()
	ctx := context.Background()

	// two pinned parents below staging with a small tree in the first
	a, _, err := createPinnedDirExclusive(layout.staging, "a", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, _, err := createPinnedDirExclusive(layout.staging, "b", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	tree, treeID, err := createPinnedDirExclusive(a, "tree", 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.InternalDir(), "staging", "a", "tree", "SKILL.md"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := source.TreeDigestRoot(ctx, tree)
	tree.Close()
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300; i++ {
		if _, err := movePinned(ctx, layout, a, "tree", b, "tree", treeID, digest); err != nil {
			t.Fatalf("forward move %d: %v", i, err)
		}
		if _, err := movePinned(ctx, layout, b, "tree", a, "tree", treeID, digest); err != nil {
			t.Fatalf("return move %d: %v", i, err)
		}
	}
	if data, rerr := os.ReadFile(filepath.Join(s.InternalDir(), "staging", "a", "tree", "SKILL.md")); rerr != nil || string(data) != "content" {
		t.Fatalf("the moved tree must survive: %q, %v", data, rerr)
	}
}
