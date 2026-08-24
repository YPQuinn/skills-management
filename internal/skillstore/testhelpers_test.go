package skillstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"skillctl/internal/source"
)

func mustFileID(t *testing.T, path string) fileID {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	id, err := fileIDOf(info)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// replaceFileWithSiblingCopy copies path into a sibling while the original
// still exists, then rename-overwrites it, so the name is kept but the
// inode cannot be the original (ext4 unlink-and-recreate can reuse inodes).
func replaceFileWithSiblingCopy(t *testing.T, path string) {
	t.Helper()
	orig := mustFileID(t, path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sib := path + ".copy"
	if err := os.WriteFile(sib, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sibID := mustFileID(t, sib)
	if sibID == orig {
		t.Fatal("sibling copy reused the original identity")
	}
	if err := os.Rename(sib, path); err != nil {
		t.Fatal(err)
	}
	if got := mustFileID(t, path); got == orig || got != sibID {
		t.Fatalf("replaced file identity: got %v, orig %v, sibling %v", got, orig, sibID)
	}
}

// replaceDirWithSiblingCopy materializes src as a sibling of path while
// path still exists, then replaces path so the directory inode differs.
func replaceDirWithSiblingCopy(t *testing.T, path, src string) {
	t.Helper()
	orig := mustFileID(t, path)
	sib := path + ".foreign"
	if err := copyTreeForTest(sib, src); err != nil {
		t.Fatal(err)
	}
	sibID := mustFileID(t, sib)
	if sibID == orig {
		t.Fatal("sibling directory reused the original identity")
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sib, path); err != nil {
		t.Fatal(err)
	}
	if got := mustFileID(t, path); got == orig || got != sibID {
		t.Fatalf("replaced dir identity: got %v, orig %v, sibling %v", got, orig, sibID)
	}
}

// newStore returns a Store rooted at a fresh temp directory.
func newStore(t *testing.T) Store {
	t.Helper()
	return New(filepath.Join(t.TempDir(), "store"))
}

// liveDigestOf computes the canonical digest of a live skill tree.
func liveDigestOf(t *testing.T, s Store, slug string) string {
	t.Helper()
	dir, err := s.SkillDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := source.TreeDigest(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// importAndFinalize runs a full import through Finalize, returning the
// committed Skill id for baseline/previous path assertions.
func importAndFinalize(t *testing.T, s Store, opID int64, slug string, m materialized, skillID int64) {
	t.Helper()
	op := Operation{ID: opID, Slug: slug, Kind: KindImport, NewDigest: m.digest}
	stageAndInstall(t, s, op, m)
	op.SkillID = skillID
	if err := s.Finalize(context.Background(), op); err != nil {
		t.Fatal(err)
	}
}

// liveDir resolves one live Skill directory, failing the test on an
// invalid slug.
func liveDir(t *testing.T, s Store, slug string) string {
	t.Helper()
	dir, err := s.SkillDir(slug)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
