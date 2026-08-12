package skillstore

import (
	"os"
	"strconv"
)

// removeOpChildren removes every allowlisted child of one operation's
// staging directory except the retained install proof (keep), after a
// strict two-phase verification with zero mutation before any child is
// proven: the entry count must match the allowlist, every child must carry
// the expected physical identity and be a regular file or an
// already-empty directory, and any other child blocks cleanup with
// ErrAmbiguous before the proof or any known evidence is removed. Phase 2
// detaches each verified child into a unique trash slot and deletes only
// the re-verified captured object. The install proof and its operation
// directory stay in place, so the caller's final live proof still runs
// with its evidence: a failure — including a byte-identical foreign live
// tree refused by that final binding — preserves the proof for
// conservative recovery.
func (l *storeLayout) removeOpChildren(opDir *os.Root, opID int64, allow map[string]fileID, keep string) error {
	if err := l.verifyOpDir(opDir, opID); err != nil {
		return err
	}
	if l.hook != nil {
		l.hook(HookBeforeRemoveOp)
	}
	// Phase 1: full scan and allowlist/identity verification. Nothing is
	// mutated before every child is proven, so an unknown child or a
	// mismatch refuses with zero mutation and the known evidence (the
	// install proof) is preserved.
	dir, err := opDir.Open(".")
	if err != nil {
		return errWrap(ErrAmbiguous, "operation directory %d cannot be listed: %v", opID, err)
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return errWrap(ErrAmbiguous, "operation directory %d cannot be listed: %v", opID, err)
	}
	if len(entries) != len(allow) {
		return errWrap(ErrAmbiguous, "operation directory %d contains evidence outside the allowed set; it is preserved", opID)
	}
	type verifiedChild struct {
		name string
		id   fileID
	}
	var children []verifiedChild
	for _, e := range entries {
		want, ok := allow[e.Name()]
		if !ok {
			return errWrap(ErrAmbiguous, "operation directory %d contains unknown child %q; it is preserved", opID, e.Name())
		}
		info, err := opDir.Lstat(e.Name())
		if err != nil {
			return errWrap(ErrAmbiguous, "operation directory %d child %q cannot be identified: %v", opID, e.Name(), err)
		}
		id, err := fileIDOf(info)
		if err != nil {
			return errWrap(ErrAmbiguous, "operation directory %d child %q cannot be identified: %v", opID, e.Name(), err)
		}
		if id != want {
			return errWrap(ErrAmbiguous, "operation directory %d child %q is not the retained object; it is preserved", opID, e.Name())
		}
		if info.IsDir() {
			sub, err := opDir.Open(e.Name())
			if err != nil {
				return errWrap(ErrAmbiguous, "operation directory %d child %q cannot be listed: %v", opID, e.Name(), err)
			}
			left, err := sub.ReadDir(-1)
			sub.Close()
			if err != nil {
				return errWrap(ErrAmbiguous, "operation directory %d child %q cannot be listed: %v", opID, e.Name(), err)
			}
			if len(left) > 0 {
				return errWrap(ErrAmbiguous, "operation directory %d child %q is not empty; it is preserved", opID, e.Name())
			}
		} else if !info.Mode().IsRegular() {
			return errWrap(ErrAmbiguous, "operation directory %d child %q is not a regular file; it is preserved", opID, e.Name())
		}
		children = append(children, verifiedChild{name: e.Name(), id: id})
	}
	// Phase 2: every verified child except the retained proof is detached
	// into a unique trash slot and only the re-verified captured object is
	// deleted.
	var seq uint64
	for _, c := range children {
		if c.name == keep {
			continue
		}
		if err := deleteVerified(opDir, c.name, c.id, &seq); err != nil {
			return err
		}
	}
	return nil
}

// removeOp is the recovery-side operation-directory cleanup: the children
// are removed while the install proof stays in place and the proof and the
// empty operation directory are then removed. Unlike Finalize, Restore
// performs no live re-binding between the two steps — the operation is
// being rolled back, not certified — but the split keeps the proof and
// operation directory in place until every allowed child is provably gone,
// so a failure preserves the evidence.
func (l *storeLayout) removeOp(opDir *os.Root, opID int64, allow map[string]fileID) error {
	proofID, ok := allow["proof"]
	if !ok {
		return errWrap(ErrAmbiguous, "operation %d allowlist has no install proof; it is preserved", opID)
	}
	if err := l.removeOpChildren(opDir, opID, allow, "proof"); err != nil {
		return err
	}
	return l.removeOpFinal(opDir, opID, proofID)
}

// removeOpFinal removes the retained install proof and the now-empty
// operation directory itself through the pinned staging handle, each only
// after its re-verification through deleteVerified. The logical entry must
// be absent after the removal, and the global layout must still identify
// its pinned handles: a foreign object that reappeared at the operation
// name, or a detached layout, is reported as ErrAmbiguous so the journal
// is never cleared for a cleanup that cannot be proven.
func (l *storeLayout) removeOpFinal(opDir *os.Root, opID int64, proofID fileID) error {
	if err := l.verifyOpDir(opDir, opID); err != nil {
		return err
	}
	var seq uint64
	if err := deleteVerified(opDir, "proof", proofID, &seq); err != nil {
		return err
	}
	opDirID, err := rootID(opDir)
	if err != nil {
		return errWrap(ErrAmbiguous, "operation directory %d cannot be identified: %v", opID, err)
	}
	name := strconv.FormatInt(opID, 10)
	if err := deleteVerified(l.staging, name, opDirID, &seq); err != nil {
		return err
	}
	if l.hook != nil {
		l.hook(HookAfterRemoveOp)
	}
	// The logical entry must be gone after the removal: a re-appeared
	// operation directory is foreign activity and the journal may not be
	// cleared for it.
	if exists, err := childExists(l.staging, name); err != nil {
		return err
	} else if exists {
		return errWrap(ErrAmbiguous, "operation directory %d reappeared after it was removed", opID)
	}
	return l.verify()
}
