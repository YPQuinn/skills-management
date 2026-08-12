package skillstore

import (
	"context"
	"path/filepath"
	"testing"

	"skillctl/internal/source"
)

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
