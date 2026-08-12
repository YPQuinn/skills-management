package skillstore

import (
	"errors"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// stageOwnership is the private creation record of one Stage invocation:
// every node the invocation created below the staging root, keyed by its
// path relative to that root, with the physical identity sampled from the
// creation handle. It is a concrete per-invocation manifest, not a generic
// filesystem abstraction: cleanup deletes exactly the recorded nodes in
// leaf→root order and refuses anything it cannot attribute to this
// invocation. The manifest travels with the opaque StagedProof, so a
// pre-install discard after an Install failure removes the exact objects
// this invocation created.
type stageOwnership struct {
	ids map[string]fileID
}

// newStageOwnership returns an empty manifest.
func newStageOwnership() *stageOwnership {
	return &stageOwnership{ids: map[string]fileID{}}
}

// record binds the created node at rel (relative to the staging root) to
// the identity sampled from its creation handle.
func (o *stageOwnership) record(rel string, id fileID) {
	o.ids[rel] = id
}

// cleanupOwnership removes every recorded node leaf→root through pinned
// parents, requiring each entry to still carry the recorded identity and
// every directory to be empty after its recorded children were removed:
// a missing expected node, an identity or name mismatch, or an unknown
// child blocks the cleanup with ErrAmbiguous and preserves everything. The
// operation directory must be finally absent. Every removal runs through
// deleteVerified, so a logical name is never unlinked directly: the entry
// is detached into a unique trash slot and only the re-verified captured
// object is deleted. The deterministic hook runs before the first removal
// and after the last removal, before the absence re-check.
func cleanupOwnership(staging *os.Root, opID int64, own *stageOwnership, hook func(HookPoint)) error {
	if own == nil || len(own.ids) == 0 {
		return errWrap(ErrAmbiguous, "operation %d has no creation manifest; nothing may be removed", opID)
	}
	keys := make([]string, 0, len(own.ids))
	for k := range own.ids {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		di, dj := strings.Count(keys[i], "/"), strings.Count(keys[j], "/")
		if di != dj {
			return di > dj
		}
		return keys[i] > keys[j]
	})
	if hook != nil {
		hook(HookBeforeRemoveOp)
	}
	var seq uint64
	for _, key := range keys {
		parentRel, name := splitRel(key)
		parent, err := ownedParent(staging, parentRel, own)
		if err != nil {
			return errWrap(ErrAmbiguous, "operation %d cleanup cannot reach %q: %v", opID, parentRel, err)
		}
		if err := removeOwnedEntry(parent, name, own.ids[key], &seq); err != nil {
			if parentRel != "" {
				parent.Close()
			}
			return errWrap(ErrAmbiguous, "operation %d cleanup refuses %q: %v", opID, key, err)
		}
		if parentRel != "" {
			parent.Close()
		}
	}
	if hook != nil {
		hook(HookAfterRemoveOp)
	}
	opName := strconv.FormatInt(opID, 10)
	if exists, err := childExists(staging, opName); err != nil {
		return err
	} else if exists {
		return errWrap(ErrAmbiguous, "operation directory %d reappeared after its cleanup", opID)
	}
	return nil
}

// ownedParent opens the recorded ancestor chain of the key parentRel below
// the staging root: every component must still carry the identity recorded
// at creation, so a foreign directory swapped in after the invocation is
// refused before any of its children can be removed. The deepest root stays
// open for the caller.
func ownedParent(staging *os.Root, parentRel string, own *stageOwnership) (*os.Root, error) {
	cur := staging
	if parentRel == "" {
		return cur, nil
	}
	parts := strings.Split(parentRel, "/")
	soFar := ""
	for i, part := range parts {
		if soFar == "" {
			soFar = part
		} else {
			soFar = path.Join(soFar, part)
		}
		want, ok := own.ids[soFar]
		if !ok {
			return nil, errWrap(ErrAmbiguous, "%q was not created by this invocation", soFar)
		}
		child, id, err := openPinnedChild(cur, part)
		if err != nil {
			if cur != staging {
				cur.Close()
			}
			return nil, err
		}
		if id != want {
			child.Close()
			if cur != staging {
				cur.Close()
			}
			return nil, errWrap(ErrAmbiguous, "%q no longer refers to the object this invocation created", soFar)
		}
		if i > 0 {
			cur.Close()
		}
		cur = child
	}
	return cur, nil
}

// removeOwnedEntry removes the single-component entry name below the pinned
// parent, requiring it to still carry the recorded identity: a missing
// node, a swapped node, a non-regular non-directory, or a directory that
// still contains children (unknown children not created by this invocation)
// is refused and preserved. The removal runs through deleteVerified, so
// the recorded name itself is never unlinked: the entry is detached into a
// unique trash slot and only the re-verified captured object is deleted.
func removeOwnedEntry(parent *os.Root, name string, want fileID, seq *uint64) error {
	info, err := parent.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errWrap(ErrAmbiguous, "%s is missing, not the created object", name)
		}
		return err
	}
	id, err := fileIDOf(info)
	if err != nil {
		return err
	}
	if id != want {
		return errWrap(ErrAmbiguous, "%s no longer refers to the object this invocation created", name)
	}
	if info.IsDir() {
		dir, err := parent.Open(name)
		if err != nil {
			return err
		}
		entries, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return errWrap(ErrAmbiguous, "%s contains children this invocation did not create", name)
		}
	} else if !info.Mode().IsRegular() {
		return errWrap(ErrAmbiguous, "%s is not a regular file this invocation created", name)
	}
	return deleteVerified(parent, name, id, seq)
}

// splitRel splits one manifest key into its parent path and final name;
// the operation directory itself has an empty parent path.
func splitRel(rel string) (parent, name string) {
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return "", rel
	}
	return rel[:i], rel[i+1:]
}
