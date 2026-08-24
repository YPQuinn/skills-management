package distribution

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"skillctl/internal/source"
)

var ErrInspection = errors.New("Target inspection could not complete")

type Relation struct {
	Slug        string
	LedgerRaw   string
	LedgerDev   uint64
	LedgerIno   uint64
	LedgerMtime int64
}

type Entry struct {
	Slug            string
	Observed        string
	Managed         bool
	Adoptable       bool
	NodeKind        string
	RawTarget       string
	ResolvedTarget  string
	ResolutionError string
}

const (
	KindFile    = "file"
	KindDir     = "dir"
	KindSymlink = "symlink"
	KindOther   = "other"
)

func Inspect(containerPath string, relations []Relation, storeRoot string) ([]Entry, error) {
	container, err := openContainerHandle(containerPath, false)
	if err != nil {
		return nil, fmt.Errorf("%w: opening %s: %v", ErrInspection, containerPath, err)
	}
	defer container.Close()
	out := make([]Entry, 0, len(relations))
	for _, rel := range relations {
		e, err := inspectEntry(container, rel, storeRoot)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func ExpectedPath(storeRoot, slug string) string { return filepath.Join(storeRoot, slug) }

// owns reports whether the ledger still names this exact symlink: the
// recorded raw target matches and the recorded physical identity is the
// current inode and mtime. A replacement with the same raw target is a
// different object even when the filesystem reuses the inode. A migrated
// row with zero identity cannot prove ownership.
func (rel Relation) owns(raw string, dev, ino uint64, mtime int64) bool {
	return rel.LedgerRaw != "" && rel.LedgerRaw == raw &&
		rel.LedgerDev != 0 && rel.LedgerIno != 0 &&
		rel.LedgerDev == dev && rel.LedgerIno == ino &&
		rel.LedgerMtime == mtime
}

func inspectEntry(container *containerHandle, rel Relation, storeRoot string) (Entry, error) {
	e := Entry{Slug: rel.Slug}
	st, err := container.stat(rel.Slug)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			e.Observed = ObservedMissing
			return e, nil
		}
		return Entry{}, fmt.Errorf("%w: %s: %v", ErrInspection, rel.Slug, err)
	}
	e.NodeKind = statKind(st)
	if e.NodeKind == KindSymlink {
		dev, ino, mtime := symlinkIdentity(st)
		return inspectSymlink(container, e, rel, storeRoot, dev, ino, mtime)
	}
	e.Observed = ObservedConflict
	return e, nil
}

func inspectSymlink(container *containerHandle, e Entry, rel Relation, storeRoot string, dev, ino uint64, mtime int64) (Entry, error) {
	raw, err := container.readlink(rel.Slug)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: reading %s: %v", ErrInspection, rel.Slug, err)
	}
	e.RawTarget = raw
	e.Managed = rel.owns(raw, dev, ino, mtime)
	expected := ExpectedPath(storeRoot, rel.Slug)
	matches, matchErr := container.targetMatches(raw, expected)
	if matchErr != nil {
		e.ResolutionError = matchErr.Error()
		if e.Managed {
			e.Observed = ObservedBrokenLink
		} else {
			e.Observed = ObservedConflict
		}
		return e, nil
	}
	if !matches {
		e.ResolutionError = "the link resolves outside the Skill Store"
		if e.Managed {
			e.Observed = ObservedBrokenLink
		} else {
			e.Observed = ObservedConflict
		}
		return e, nil
	}
	e.ResolvedTarget = expected
	if err := source.ValidateSkillDir(expected); err != nil {
		e.ResolutionError = err.Error()
		if e.Managed {
			e.Observed = ObservedBrokenLink
		} else {
			e.Observed = ObservedConflict
		}
		return e, nil
	}
	if e.Managed {
		e.Observed = ObservedLinked
	} else {
		e.Observed = ObservedConflict
		e.Adoptable = true
	}
	return e, nil
}
