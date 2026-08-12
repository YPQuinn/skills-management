package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/domain"
)

// sparseFile creates a file with the given logical size using no disk.
func sparseFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestMaterializeLocalGuardPerFileLimit proves the per-file guard rejects
// before the file's bytes are copied.
func TestMaterializeLocalGuardPerFileLimit(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	sparseFile(t, filepath.Join(root, "skills", "alpha", "big.bin"), domain.MaxBytesPerFile+1)

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	dst := t.TempDir()
	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", dst, false)
	if err == nil || !strings.Contains(err.Error(), "per-file limit") {
		t.Fatalf("per-file guard: got %v, want a limit error", err)
	}
	entries, readErr := os.ReadDir(dst)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("guard preflight must reject before copying any file: %v, %v", entries, readErr)
	}
}

// TestMaterializeLocalGuardFileCount proves the count guard trips while
// copying, before the remaining files are read.
func TestMaterializeLocalGuardFileCount(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	dir := filepath.Join(root, "skills", "alpha", "many")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= domain.MaxFilesPerSkill; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "file limit") {
		t.Fatalf("file-count guard: got %v, want a limit error", err)
	}
}

// TestMaterializeLocalGuardTotalLimit proves the total-byte guard trips
// during the walk even when every single file is below the per-file limit.
func TestMaterializeLocalGuardTotalLimit(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	sparseFile(t, filepath.Join(root, "skills", "alpha", "a.bin"), domain.MaxBytesPerSkill/2)
	sparseFile(t, filepath.Join(root, "skills", "alpha", "b.bin"), domain.MaxBytesPerSkill/2)

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	_, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "total limit") {
		t.Fatalf("total guard: got %v, want a limit error", err)
	}
}

// TestMaterializeLocalAllowLargeBypassesGuards proves allow-large skips the
// limits while unsafe nodes are still rejected (covered by the staging and
// symlink tests).
func TestMaterializeLocalAllowLargeBypassesGuards(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	sparseFile(t, filepath.Join(root, "skills", "alpha", "big.bin"), domain.MaxBytesPerFile+1)

	_, entry := observeLocalEntry(t, root, "skills/alpha")
	dst := t.TempDir()
	if _, err := MaterializeEntry(context.Background(), Locator{Kind: KindLocal, Location: root}, "", entry, "", dst, true); err != nil {
		t.Fatalf("allow-large: %v", err)
	}
	if got, err := TreeDigest(context.Background(), dst); err != nil || got != entry.Digest {
		t.Fatalf("allow-large digest: %s vs %s (%v)", got, entry.Digest, err)
	}
}
