//go:build darwin || linux

package source

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"testing"
)

// TestRootedWriteLowFDRegression proves the symlink-swap failure path never
// leaks the still-open created handle: with a tight RLIMIT_NOFILE and GC
// disabled — the in-process equivalent of GOGC=off, so no os.File
// finalizer can paper over a leak — enough iterations of the real
// rootedWriteFile/streamCopyRooted seams must never fail with "too many
// open files". RLIMIT and GC are restored before the test returns; the
// test stays sequential (no t.Parallel) because the limit is process-wide.
func TestRootedWriteLowFDRegression(t *testing.T) {
	// GOGC=off equivalent: while the GC is off, a created handle leaked on
	// the chmod failure keeps its FD forever, so the low limit below is
	// exhausted deterministically instead of being reclaimed by a finalizer.
	prevGC := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(prevGC)

	var rl syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		t.Fatal(err)
	}
	low := rl
	if low.Cur > 128 {
		low.Cur = 128
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &low); err != nil {
		t.Fatal(err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rl)

	t.Run("rootedWriteFile", func(t *testing.T) {
		dir := t.TempDir()
		dst, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer dst.Close()
		target := filepath.Join(t.TempDir(), "target.txt")
		if err := os.WriteFile(target, []byte("foreign"), 0o600); err != nil {
			t.Fatal(err)
		}
		defer func() { testBeforeChmodFileHook = nil }()
		testBeforeChmodFileHook = swapFileHook(t, dir, "f.txt", target)
		content := []byte("data")
		runLowFDSwapLoop(t, 512, func() error {
			return rootedWriteFile(context.Background(), dst, "f.txt", content, 0o644)
		})
		assertForeignUntouched(t, target, "foreign")
	})

	t.Run("streamCopyRooted", func(t *testing.T) {
		dir := t.TempDir()
		dst, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer dst.Close()
		src := filepath.Join(t.TempDir(), "src.bin")
		if err := os.WriteFile(src, bytes.Repeat([]byte("s"), 256<<10), 0o600); err != nil {
			t.Fatal(err)
		}
		in, err := os.Open(src)
		if err != nil {
			t.Fatal(err)
		}
		defer in.Close()
		target := filepath.Join(t.TempDir(), "target.bin")
		if err := os.WriteFile(target, []byte("foreign"), 0o600); err != nil {
			t.Fatal(err)
		}
		defer func() { testBeforeChmodFileHook = nil }()
		testBeforeChmodFileHook = swapFileHook(t, dir, "big.bin", target)
		runLowFDSwapLoop(t, 128, func() error {
			if _, err := in.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			return streamCopyRooted(context.Background(), in, dst, "big.bin", 0o644)
		})
		assertForeignUntouched(t, target, "foreign")
	})
}

// swapFileHook returns the symlink-swap seam used by the existing chmod
// tests: the created file is renamed aside and a symlink to the foreign
// target is planted on its name, so the mode can never be applied to the
// target and the write must fail with a name-mismatch error.
func swapFileHook(t *testing.T, dir, name, target string) func(*os.Root, string) {
	return func(parent *os.Root, swapName string) {
		if swapName != name {
			return
		}
		if err := os.Rename(filepath.Join(dir, name), filepath.Join(dir, name+".real")); err != nil {
			t.Errorf("hook rename %s: %v", name, err)
			return
		}
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Errorf("hook symlink %s: %v", name, err)
		}
	}
}

// runLowFDSwapLoop runs fn until it has failed iterations times with the
// expected name-mismatch error, failing the test on EMFILE — the symptom
// of a created handle leaked across the chmod failure — or on any other
// unexpected outcome.
func runLowFDSwapLoop(t *testing.T, iterations int, fn func() error) {
	t.Helper()
	for i := 0; i < iterations; i++ {
		err := fn()
		if err == nil {
			t.Fatalf("iteration %d: symlink swap before the chmod must fail the write", i)
		}
		if errors.Is(err, syscall.EMFILE) {
			t.Fatalf("iteration %d: too many open files: %v — the created handle leaked across the chmod failure", i, err)
		}
		if !strings.Contains(err.Error(), "changed while its mode was set") {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}
}

// assertForeignUntouched proves the symlink-swap target was never chmod'd
// or written through.
func assertForeignUntouched(t *testing.T, target, want string) {
	t.Helper()
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("foreign target mode must be unchanged: got %o, want 600", info.Mode().Perm())
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != want {
		t.Fatalf("foreign target content must be unchanged: %q, %v", data, err)
	}
}
