package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

// Node kinds shared with the canonical tree identity vocabulary.
const (
	KindFile        = "file"
	KindDir         = "dir"
	KindSymlink     = "symlink"
	KindFIFO        = "fifo"
	KindSocket      = "socket"
	KindBlockDevice = "block_device"
	KindCharDevice  = "char_device"
	KindOther       = "other"
)

// Presentation limits for unified text differences (decision 05): ordinary
// UTF-8 text without NUL bytes renders a unified difference only while both
// sides stay inside these bounds; larger or non-text files expose paths,
// sizes, and digests instead.
const (
	TextMaxBytes = 256 << 10
	TextMaxLines = 2000
)

// Node is one tree entry of a compared tree: its exact kind, the executable
// bit (regular files and directories), the file size, and its content
// identity (SHA-256 of file bytes, or of the symlink target string).
type Node struct {
	Kind   string `json:"kind"`
	Exec   *bool  `json:"exec,omitempty"`
	Size   *int64 `json:"size,omitempty"`
	Digest string `json:"digest,omitempty"`
	// Text carries the file's decoded text when the file qualifies for a
	// unified rendering (UTF-8, no NUL, within the presentation limits).
	Text *string `json:"-"`
}

// Tree is one complete snapshot of a Skill tree keyed by Skill-relative
// path; the root directory is ".".
type Tree map[string]Node

// SnapshotTree walks dir once and captures every node with its identity and
// optional text. Symlinks are recorded by target and never followed; Source
// .git metadata is excluded exactly like the canonical digest walk. The
// caller already holds the canonical digests from observation and SQLite;
// this snapshot exists purely for path-level comparison and presentation.
func SnapshotTree(ctx context.Context, dir string) (Tree, error) {
	t := Tree{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		n, err := snapshotNode(ctx, path, d)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		t[rel] = n
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// SortedPaths returns every path of the tree in lexical order.
func SortedPaths(t Tree) []string {
	out := make([]string, 0, len(t))
	for p := range t {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// snapshotNode captures one walk entry.
func snapshotNode(ctx context.Context, path string, d fs.DirEntry) (Node, error) {
	info, err := d.Info()
	if err != nil {
		return Node{}, err
	}
	exec := info.Mode()&0o111 != 0
	switch {
	case d.IsDir():
		return Node{Kind: KindDir, Exec: &exec}, nil
	case d.Type()&fs.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return Node{}, err
		}
		sum := sha256.Sum256([]byte(target))
		return Node{Kind: KindSymlink, Digest: hex.EncodeToString(sum[:])}, nil
	case d.Type().IsRegular():
		return snapshotFile(ctx, path, info, exec)
	default:
		return Node{Kind: walkKind(d.Type())}, nil
	}
}

// snapshotFile hashes one regular file and captures its text when it
// qualifies for unified rendering.
func snapshotFile(ctx context.Context, path string, info fs.FileInfo, exec bool) (Node, error) {
	f, err := os.Open(path)
	if err != nil {
		return Node{}, err
	}
	defer f.Close()
	size := info.Size()
	n := Node{Kind: KindFile, Exec: &exec, Size: &size}
	if size > TextMaxBytes {
		digest, err := hashReader(ctx, f)
		if err != nil {
			return Node{}, err
		}
		n.Digest = digest
		return n, nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return Node{}, err
	}
	sum := sha256.Sum256(data)
	n.Digest = hex.EncodeToString(sum[:])
	if utf8.Valid(data) && bytes.IndexByte(data, 0) == -1 && bytes.Count(data, []byte("\n")) <= TextMaxLines {
		text := string(data)
		n.Text = &text
	}
	return n, nil
}

func hashReader(ctx context.Context, r io.Reader) (string, error) {
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := r.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err == io.EOF {
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		if err != nil {
			return "", err
		}
	}
}

// walkKind classifies a special node by its exact type without opening it.
func walkKind(t fs.FileMode) string {
	switch {
	case t&fs.ModeNamedPipe != 0:
		return KindFIFO
	case t&fs.ModeSocket != 0:
		return KindSocket
	case t&fs.ModeCharDevice != 0:
		return KindCharDevice
	case t&fs.ModeDevice != 0:
		return KindBlockDevice
	default:
		return KindOther
	}
}
