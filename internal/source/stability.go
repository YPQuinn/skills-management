package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// defaultLocalStabilityInterval is the pause between consecutive Local
// observation scans. It is deliberately short: a Source that is still
// changing after one retry is reported unavailable rather than observed
// mid-flight.
const defaultLocalStabilityInterval = 200 * time.Millisecond

// snapshotNodeKind is the exact node type of one snapshotted path. Special
// node types (FIFO, socket, block or character device) are recorded by type
// and never opened, so a snapshot can never block on or read their content.
type snapshotNodeKind string

const (
	kindDir         snapshotNodeKind = "dir"
	kindFile        snapshotNodeKind = "file"
	kindSymlink     snapshotNodeKind = "symlink"
	kindFIFO        snapshotNodeKind = "fifo"
	kindSocket      snapshotNodeKind = "socket"
	kindBlockDevice snapshotNodeKind = "block_device"
	kindCharDevice  snapshotNodeKind = "char_device"
	kindOther       snapshotNodeKind = "other"
)

// snapshotKind classifies a non-directory node by its exact type without
// opening it. ModeCharDevice is checked before ModeDevice because Go sets
// both bits for character devices.
func snapshotKind(t fs.FileMode) snapshotNodeKind {
	switch {
	case t&fs.ModeSymlink != 0:
		return kindSymlink
	case t.IsRegular():
		return kindFile
	case t&fs.ModeNamedPipe != 0:
		return kindFIFO
	case t&fs.ModeSocket != 0:
		return kindSocket
	case t&fs.ModeCharDevice != 0:
		return kindCharDevice
	case t&fs.ModeDevice != 0:
		return kindBlockDevice
	default:
		return kindOther
	}
}

// snapshot is the filesystem state of a Local Source's observation scope:
// every node of every discovered Skill candidate directory, recursively,
// with node type, size, executable bit, and content identity. Directories
// record their executable bit; regular files record size, executable bit,
// and content digest; symlinks record their target string; special nodes
// are recorded by kind only and never opened. Source .git metadata
// directories are excluded. Two snapshots are equal exactly when the
// observation scope is unchanged.
type snapshot struct {
	entries []snapshotEntry
}

type snapshotEntry struct {
	relPath    string
	kind       snapshotNodeKind
	size       int64
	exec       bool
	digest     string
	linkTarget string
}

// snapshotRoot records the complete content of every Skill candidate
// directory under root, using the same candidate discovery as Discover.
// Entries are sorted by path for deterministic comparison and digesting.
// Discovery and hashing honor ctx cancellation.
func snapshotRoot(ctx context.Context, root string) (snapshot, error) {
	dirs, err := skillCandidates(ctx, root)
	if err != nil {
		return snapshot{}, err
	}
	var snap snapshot
	for _, dir := range dirs {
		base, err := filepath.Rel(root, dir)
		if err != nil {
			return snapshot{}, err
		}
		if err := snapshotTree(ctx, dir, filepath.ToSlash(base), &snap); err != nil {
			return snapshot{}, err
		}
	}
	sort.Slice(snap.entries, func(i, j int) bool { return snap.entries[i].relPath < snap.entries[j].relPath })
	return snap, nil
}

// snapshotTree appends every node below dir (including dir itself) to snap,
// with paths prefixed by base (relative to the Source root; "." for the
// root candidate). Only Source .git metadata directories are excluded; all
// other content, however deep, is part of the Skill's tree.
func snapshotTree(ctx context.Context, dir, base string, snap *snapshot) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)
		if base != "." {
			if rel == "." {
				relPath = base
			} else {
				relPath = base + "/" + relPath
			}
		}
		return snap.addEntry(ctx, path, relPath, d)
	})
}

func (snap *snapshot) addEntry(ctx context.Context, path, relPath string, d fs.DirEntry) error {
	e := snapshotEntry{relPath: relPath}
	if d.IsDir() {
		e.kind = kindDir
		info, err := d.Info()
		if err != nil {
			return err
		}
		// A directory's executable (search) bit is content: it decides
		// whether the tree is traversable. Its byte size is filesystem
		// metadata that churns with unrelated directory operations, so it
		// is deliberately not part of the content identity.
		e.exec = info.Mode()&0o111 != 0
		snap.entries = append(snap.entries, e)
		return nil
	}
	e.kind = snapshotKind(d.Type())
	switch e.kind {
	case kindSymlink:
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		e.linkTarget = target
	case kindFile:
		info, err := d.Info()
		if err != nil {
			return err
		}
		e.size = info.Size()
		e.exec = info.Mode()&0o111 != 0
		sum, err := hashFileCtx(ctx, path)
		if err != nil {
			return err
		}
		e.digest = sum
	}
	snap.entries = append(snap.entries, e)
	return nil
}

func (a snapshot) equal(b snapshot) bool {
	if len(a.entries) != len(b.entries) {
		return false
	}
	for i := range a.entries {
		if a.entries[i] != b.entries[i] {
			return false
		}
	}
	return true
}

// snapshotDigest returns the canonical content digest of one Skill
// candidate from its snapshot entries, with paths relative to the Skill
// directory. The canonical node serialization is shared with the Git
// observer, so an equivalent materialized tree yields the same digest from
// both Source kinds. Directory executable bits are stability state only and
// are not part of the canonical digest, because Git cannot represent them.
func snapshotDigest(snap snapshot, base string) string {
	var nodes []canonNode
	for _, e := range snap.entries {
		rel := e.relPath
		if base != "." {
			if rel == base {
				rel = "."
			} else if strings.HasPrefix(rel, base+"/") {
				rel = strings.TrimPrefix(rel, base+"/")
			} else {
				continue
			}
		}
		n := canonNode{relPath: rel, kind: string(e.kind)}
		switch e.kind {
		case kindFile:
			n.exec = e.exec
			n.content = e.digest
		case kindSymlink:
			n.content = e.linkTarget
		}
		nodes = append(nodes, n)
	}
	return canonDigest(nodes)
}

// hashFileCtx returns the hex SHA-256 of a file's bytes, checking ctx
// between chunks so a cancelled observation aborts promptly even while
// hashing a large file.
func hashFileCtx(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
		}
		if rerr == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if rerr != nil {
			return "", rerr
		}
	}
}

// localScan is one Local observation attempt: the inventory and the content
// snapshot it was derived from.
type localScan struct {
	obs  Observation
	snap snapshot
}

// observeStable scans a Local Source twice and accepts the observation only
// when both scans agree on inventory and content. An unstable tree receives
// one short retry; continued change fails the whole observation so the
// caller keeps the previous Inventory as stale.
func observeStable(ctx context.Context, interval time.Duration, scan func(context.Context) (localScan, error)) (Observation, error) {
	first, err := scan(ctx)
	if err != nil {
		return Observation{}, err
	}
	if err := waitStability(ctx, interval); err != nil {
		return Observation{}, err
	}
	second, err := scan(ctx)
	if err != nil {
		return Observation{}, err
	}
	if scansAgree(first, second) {
		return second.obs, nil
	}
	if err := waitStability(ctx, interval); err != nil {
		return Observation{}, err
	}
	third, err := scan(ctx)
	if err != nil {
		return Observation{}, err
	}
	if scansAgree(second, third) {
		return third.obs, nil
	}
	return Observation{}, fmt.Errorf("Source changed while it was being checked; retry when it is stable")
}

func scansAgree(a, b localScan) bool {
	return a.snap.equal(b.snap) && reflect.DeepEqual(a.obs, b.obs)
}

func waitStability(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
