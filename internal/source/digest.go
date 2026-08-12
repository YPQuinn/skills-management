package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// Canonical node kinds shared by the Local and Git tree digests. A node's
// kind is its exact type; exec is the executable bit (regular files only);
// content is the node's content identity: the SHA-256 of the file bytes for
// regular files, the raw target bytes for symlinks, the referenced commit
// object id for gitlinks, and empty for directories and other special nodes.
const (
	canonKindDir         = "dir"
	canonKindFile        = "file"
	canonKindSymlink     = "symlink"
	canonKindGitlink     = "gitlink"
	canonKindFIFO        = "fifo"
	canonKindSocket      = "socket"
	canonKindBlockDevice = "block_device"
	canonKindCharDevice  = "char_device"
	canonKindOther       = "other"
)

// canonNode is one node of a Skill tree in canonical form: its Skill-
// relative path, kind, executable bit, and content identity.
type canonNode struct {
	relPath string
	kind    string
	exec    bool
	content string
}

// canonDigest is the deterministic SHA-256 of a Skill tree: every node with
// its Skill-relative path, canonical kind, executable bit, and content
// identity, serialized length-prefixed so paths and content may contain any
// bytes. It never includes the Source location, the configured subpath, Git
// object ids, or cache paths, so an equivalent materialized tree produces
// the same digest from a Local Source and a Git Source.
func canonDigest(nodes []canonNode) string {
	sorted := append([]canonNode(nil), nodes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].relPath < sorted[j].relPath })
	h := sha256.New()
	for _, n := range sorted {
		exec := 0
		if n.exec {
			exec = 1
		}
		fmt.Fprintf(h, "%s %d %d ", n.kind, exec, len(n.content))
		h.Write([]byte(n.content))
		fmt.Fprintf(h, "%d ", len(n.relPath))
		h.Write([]byte(n.relPath))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// hashBytes returns the hex SHA-256 of b, the content identity of a regular
// file node in both Local and Git observations.
func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TreeDigest computes the canonical content digest of the directory tree
// rooted at dir, with paths relative to dir itself and .git metadata
// excluded. It is the same computation the observers use for one Skill
// entry, so a materialized copy can be verified against an observation and
// Store trees and Synchronization Baselines share one content identity.
// Symlinks are recorded as symlink nodes, never followed.
func TreeDigest(ctx context.Context, dir string) (string, error) {
	var snap snapshot
	if err := snapshotTree(ctx, dir, ".", &snap); err != nil {
		return "", err
	}
	return snapshotDigest(snap, "."), nil
}
