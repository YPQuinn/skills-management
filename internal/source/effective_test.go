package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skillctl/internal/domain"
)

// TestValidateRelPath pins the canonical-path rule every Git destination
// path must satisfy before it is touched.
func TestValidateRelPath(t *testing.T) {
	bad := []string{"", "/abs", "a/../b", "a/./b", "a//b", "../up", "a/..", "..", ".", "a/../.."}
	for _, p := range bad {
		if err := validateRelPath(p); err == nil {
			t.Errorf("validateRelPath(%q): want error", p)
		}
	}
	good := []string{"SKILL.md", "a", "a/b/c", "with space.md", "tab\tname", "ünïcode/emoji-😀"}
	for _, p := range good {
		if err := validateRelPath(p); err != nil {
			t.Errorf("validateRelPath(%q): %v", p, err)
		}
	}
}

func TestGitRelativePathRequiresCanonicalSubpathOutput(t *testing.T) {
	for _, repoPath := range []string{"other/SKILL.md", "catalog/../SKILL.md", "catalog//SKILL.md"} {
		if _, err := gitRelativePath(repoPath, "catalog"); err == nil {
			t.Errorf("gitRelativePath(%q): want error", repoPath)
		}
	}
	if got, err := gitRelativePath("catalog/skills/alpha/SKILL.md", "catalog"); err != nil || got != "skills/alpha/SKILL.md" {
		t.Fatalf("canonical output: got %q, %v", got, err)
	}
}

// TestLocalSymlinkDigestIsEffectiveIdentity builds a Local tree whose
// internal symlinks are dereferenced by materialization and proves the
// observed entry digest equals the digest of the materialized (and
// Store-shaped) tree: a newly imported symlink Skill must never diverge.
func TestLocalSymlinkDigestIsEffectiveIdentity(t *testing.T) {
	root := resolveTempDir(t)
	writeSkill(t, filepath.Join(root, "skills", "alpha"), "Alpha", "one")
	if err := os.MkdirAll(filepath.Join(root, "skills", "alpha", "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "alpha", "tools", "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// file link, directory link, and a chain through a second link
	if err := os.Symlink("tools/run.sh", filepath.Join(root, "skills", "alpha", "link-file")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tools", filepath.Join(root, "skills", "alpha", "link-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("link-file", filepath.Join(root, "skills", "alpha", "link-chain")); err != nil {
		t.Fatal(err)
	}

	obs, entry := observeLocalEntry(t, root, "skills/alpha")
	if len(obs.Issues) != 0 {
		t.Fatalf("valid symlinks must not produce issues: %+v", obs.Issues)
	}
	dst := t.TempDir()
	digest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: root}, entry, dst)

	// The digest of the dereferenced copy is the effective identity; it
	// must equal the observation digest and the digest of the same tree
	// re-walked as the Store would walk it.
	if digest != entry.Digest {
		t.Fatalf("materialized %s != observed %s", digest, entry.Digest)
	}
	storeDigest, err := TreeDigest(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	if storeDigest != entry.Digest {
		t.Fatalf("store-shaped tree %s != observed %s", storeDigest, entry.Digest)
	}
}

// TestGitSymlinkDigestIsEffectiveIdentity proves the Git observer's entry
// digest equals the digest of the dereferenced materialized tree.
func TestGitSymlinkDigestIsEffectiveIdentity(t *testing.T) {
	files := map[string]treeFile{
		"skills/alpha/SKILL.md":     {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/tools/run.sh": {content: "#!/bin/sh\n", mode: 0o755},
		"skills/alpha/link-file":    {symlink: "tools/run.sh"},
		"skills/alpha/link-dir":     {symlink: "tools"},
		"skills/alpha/link-chain":   {symlink: "link-file"},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, commit := observeGitEntry(t, loc, workDir, "skills/alpha")
	dst := t.TempDir()
	digest := materializeGitEntry(t, loc, commit, entry, workDir, dst)

	if digest != entry.Digest {
		t.Fatalf("materialized %s != observed %s", digest, entry.Digest)
	}
	storeDigest, err := TreeDigest(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	if storeDigest != entry.Digest {
		t.Fatalf("store-shaped tree %s != observed %s", storeDigest, entry.Digest)
	}
}

// TestLocalAndGitMaterializedDigestsMatch proves the same effective tree
// materialized from a Local Source and a Git Source carries one digest,
// including dereferenced symlinks, so sync can rely on one content identity
// across both Source kinds.
func TestLocalAndGitMaterializedDigestsMatch(t *testing.T) {
	files := map[string]treeFile{
		"SKILL.md":           {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"run.sh":             {content: "#!/bin/sh\necho hi\n", mode: 0o755},
		"link.md":            {symlink: "run.sh"},
		"refs/deep/nested/x": {content: "deep", mode: 0o644},
	}
	bare, work := buildRepo(t, files, "main")

	var g Git
	gitObs, err := g.Observe(context.Background(), gitLoc(t, bare, "", ""), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitEntry := gitObs.Entries[0]
	gitCommit := gitObs.Commit
	gitDst := t.TempDir()
	gitDigest := materializeGitEntry(t, gitLoc(t, bare, "", ""), gitCommit, gitEntry, t.TempDir(), gitDst)

	localObs, err := (Local{stabilityInterval: 0}).Observe(context.Background(),
		Locator{Kind: KindLocal, Location: work}, "")
	if err != nil {
		t.Fatal(err)
	}
	localDst := t.TempDir()
	localDigest := materializeLocalEntry(t, Locator{Kind: KindLocal, Location: work}, localObs.Entries[0], localDst)

	if gitDigest != localDigest {
		t.Fatalf("materialized digests differ: git %s vs local %s", gitDigest, localDigest)
	}
	if gitDigest != gitEntry.Digest || localDigest != localObs.Entries[0].Digest {
		t.Fatalf("materialized digests must equal the observed entries: git %s/%s local %s/%s",
			gitDigest, gitEntry.Digest, localDigest, localObs.Entries[0].Digest)
	}
}

// TestMaterializeGitGuardPerFileLimitPreflight proves a Git Skill with a
// file over the per-file limit is rejected from the tree sizes before any
// blob fetch.
func TestGitGuardPreflightCountsDereferencedLinks(t *testing.T) {
	rows := []treeLine{
		{mode: "100644", oid: "file", rel: "large.bin", size: domain.MaxBytesPerFile},
		{mode: "120000", oid: "link-one", rel: "one.bin", size: int64(len("large.bin"))},
		{mode: "120000", oid: "link-two", rel: "two.bin", size: int64(len("large.bin"))},
	}
	links := map[string][]byte{
		"link-one": []byte("large.bin"),
		"link-two": []byte("large.bin"),
	}
	if err := preflightGitMaterializedGuards(rows, "", links); err == nil || !strings.Contains(err.Error(), "total limit") {
		t.Fatalf("dereferenced-link preflight: got %v, want a total-limit error", err)
	}
}

func TestMaterializeGitGuardPerFileLimitPreflight(t *testing.T) {
	big := strings.Repeat("x", domain.MaxBytesPerFile+1)
	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: one\n---\n", mode: 0o644},
		"skills/alpha/big.bin":  {content: big, mode: 0o644},
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	// observation itself fetches the blob (digests need content), so the
	// entry is real; materialization must reject before fetching again
	entry, commit := observeGitEntry(t, loc, workDir, "skills/alpha")
	_, err := MaterializeEntry(context.Background(), loc, commit, entry, workDir, t.TempDir(), false)
	if err == nil || !strings.Contains(err.Error(), "per-file limit") {
		t.Fatalf("per-file preflight: got %v, want a limit error", err)
	}
	// allow-large lets the same content through
	dst := t.TempDir()
	if _, err := MaterializeEntry(context.Background(), loc, commit, entry, workDir, dst, true); err != nil {
		t.Fatalf("allow-large: %v", err)
	}
}
