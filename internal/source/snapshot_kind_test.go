package source

import (
	"io/fs"
	"testing"
)

// TestSnapshotKindClassifiesSpecialNodes pins the exact node classification
// without creating device nodes: character devices carry both ModeDevice and
// ModeCharDevice and must classify as char_device, never block_device.
func TestSnapshotKindClassifiesSpecialNodes(t *testing.T) {
	cases := []struct {
		mode fs.FileMode
		want snapshotNodeKind
	}{
		{0, kindFile},
		{fs.ModeSymlink, kindSymlink},
		{fs.ModeNamedPipe, kindFIFO},
		{fs.ModeSocket, kindSocket},
		{fs.ModeDevice, kindBlockDevice},
		{fs.ModeDevice | fs.ModeCharDevice, kindCharDevice},
		{fs.ModeIrregular, kindOther},
	}
	for _, c := range cases {
		if got := snapshotKind(c.mode); got != c.want {
			t.Errorf("snapshotKind(%v): got %q, want %q", c.mode, got, c.want)
		}
	}
}

// TestSnapshotDirectoryModeStabilityOnly proves a directory's executable
// bit is part of snapshot equality but deliberately not of the canonical
// entry digest: Git cannot represent directory modes, so including them
// would break Local/Git digest equivalence. The construction is portable:
// flipping a real directory's mode without breaking traversal is not
// possible (removing the search bit makes the tree unreadable), so the two
// states are built directly.
func TestSnapshotDirectoryModeStabilityOnly(t *testing.T) {
	base := snapshot{entries: []snapshotEntry{
		{relPath: "skills/alpha", kind: kindDir, exec: true},
		{relPath: "skills/alpha/SKILL.md", kind: kindFile, size: 5, digest: "d"},
	}}
	noexec := snapshot{entries: []snapshotEntry{
		{relPath: "skills/alpha", kind: kindDir, exec: false},
		{relPath: "skills/alpha/SKILL.md", kind: kindFile, size: 5, digest: "d"},
	}}
	if base.equal(noexec) {
		t.Fatal("a directory exec-bit difference must differ the snapshot")
	}
	if snapshotDigest(base, "skills/alpha") != snapshotDigest(noexec, "skills/alpha") {
		t.Fatal("directory exec bits are stability state only and must not change the canonical entry digest")
	}
}
