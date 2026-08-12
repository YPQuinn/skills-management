//go:build darwin || linux

package source

import (
	"fmt"
	"runtime/debug"
	"syscall"
	"testing"
)

// TestMaterializeGitDeepTreeWithinNoFileLimit proves rooted Git
// materialization never leaks directory handles. With the soft open-file
// limit lowered to 128 and the GC disabled — so a leaked *os.Root is never
// recycled by a finalizer mid-test — a valid Skill of SKILL.md plus ~300
// files across nested and sibling directories must materialize successfully
// with the observation digest. Every intermediate root opened by
// ensureDirs/rootedMkdir is closed on the success and error paths, so peak
// concurrent FDs stay far below the limit. The original rlimit and GC
// setting are restored afterwards; the test never runs in parallel.
func TestMaterializeGitDeepTreeWithinNoFileLimit(t *testing.T) {
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

	files := map[string]treeFile{
		"skills/alpha/SKILL.md": {content: "---\nname: Alpha\ndescription: fd regression\n---\n", mode: 0o644},
	}
	// a deep nested chain plus wide sibling directories: ~300 files total
	for i := 0; i < 20; i++ {
		for j := 0; j < 10; j++ {
			files[fmt.Sprintf("skills/alpha/nest/l%02d/f%d.md", i, j)] = treeFile{content: fmt.Sprintf("level %d file %d", i, j), mode: 0o644}
		}
	}
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			files[fmt.Sprintf("skills/alpha/sib%02d/f%d.md", i, j)] = treeFile{content: fmt.Sprintf("sibling %d file %d", i, j), mode: 0o644}
		}
	}
	bare, _ := buildRepo(t, files, "main")
	loc := gitLoc(t, bare, "", "")
	workDir := t.TempDir()

	entry, commit := observeGitEntry(t, loc, workDir, "skills/alpha")
	dst := t.TempDir()
	digest := materializeGitEntry(t, loc, commit, entry, workDir, dst)
	if digest != entry.Digest {
		t.Fatalf("materialized digest %s must equal the observation digest %s under the lowered file limit", digest, entry.Digest)
	}
}
