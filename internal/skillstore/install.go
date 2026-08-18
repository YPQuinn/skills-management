package skillstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"skillctl/internal/domain"
	"skillctl/internal/source"
)

// Install performs the live-path transition for one staged import or
// replace and prepares the operation's Baseline candidate inside staging.
//
// Install consumes the opaque StagedProof produced by Stage for the same
// operation: the staged tree is only accepted when its physical identity
// still matches the object Stage proved, and its digest must match both the
// proof and the journal's NewDigest. Every fallible step before the first
// live mutation — identity proofs, hashes, the exclusive Baseline-candidate
// copy (recorded in the proof's creation manifest) — returns an ordinary
// error the caller unwinds by discarding the staging with the opaque proof.
// After the first live mutation (live→recovery for a replace, staged→live
// for both kinds) every failure is ErrAmbiguous with all candidates
// preserved; the durable install proof is written only after every physical
// move completed, and a proof write failure is ErrAmbiguous because the
// moved state cannot be attributed. An import refuses when <store>/<slug>
// already exists (ErrUnmanaged) and installs with an atomic no-replace
// rename; a replace proves the live tree still carries op.OldDigest before
// moving it to recovery/<opID>.
func (s Store) Install(ctx context.Context, op Operation, staged *StagedProof) error {
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return err
	}
	if staged == nil || staged.opID != op.ID {
		return errWrap(ErrAmbiguous, "staged proof belongs to operation %d, not %d", staged.opID, op.ID)
	}
	if staged.ownership == nil || staged.treeID == (fileID{}) || staged.opDirID == (fileID{}) {
		return errWrap(ErrAmbiguous, "staged proof for operation %d is incomplete", op.ID)
	}
	if err := s.EnsureLayout(); err != nil {
		return err
	}
	layout, err := s.openLayout()
	if err != nil {
		return err
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return err
	}
	opDir, err := layout.opDir(op.ID)
	if errors.Is(err, os.ErrNotExist) {
		return errWrap(ErrAmbiguous, "operation directory %d is missing; Install cannot attribute any staging to it", op.ID)
	}
	if err != nil {
		return err
	}
	defer opDir.Close()
	if err := layout.verifyOpDir(opDir, op.ID); err != nil {
		return err
	}
	opDirID, err := rootID(opDir)
	if err != nil {
		return err
	}
	if opDirID != staged.opDirID {
		return errWrap(ErrAmbiguous, "operation directory for %d is not the object %d:%d that Stage proved", op.ID, staged.opDirID.dev, staged.opDirID.ino)
	}
	tree, err := openLayoutDir(opDir, "tree")
	if err != nil {
		return err
	}
	defer tree.Close()
	treeID, err := rootID(tree)
	if err != nil {
		return err
	}
	// Bind the opened staged tree to the object Stage proved before any
	// hash: a byte-identical foreign tree is refused by physical identity
	// even before its digest is consulted.
	if treeID != staged.treeID {
		return errWrap(ErrAmbiguous, "staged tree for operation %d is not the object %d:%d that Stage proved", op.ID, staged.treeID.dev, staged.treeID.ino)
	}
	digest, err := source.TreeDigestRoot(ctx, tree)
	if err != nil {
		return err
	}
	if digest != op.NewDigest || digest != staged.digest {
		return fmt.Errorf("staged content digest %s does not match the expected digest %s", digest, op.NewDigest)
	}
	// Test seam: a staged tree swapped in here must fail the source-name
	// re-proof below and never be installed.
	s.runHook(HookAfterStagedHash)
	if err := nameRefersTo(opDir, "tree", treeID); err != nil {
		return errWrap(ErrAmbiguous, "staged tree for operation %d changed after it was verified", op.ID)
	}
	if err := layout.verifyOpDir(opDir, op.ID); err != nil {
		return err
	}
	// Pre-mutation proof of the old live tree for a replace.
	var oldLive *os.Root
	var oldLiveID fileID
	switch op.Kind {
	case KindImport:
		if _, err := layout.store.Lstat(op.Slug); err == nil {
			return ErrUnmanaged
		} else if !os.IsNotExist(err) {
			return err
		}
	case KindReplace:
		if op.oldUnreadable() {
			// Not a digestable Skill tree: proven by identity only.
			info, err := layout.store.Lstat(op.Slug)
			if errors.Is(err, os.ErrNotExist) {
				return ErrMissing
			}
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("managed Skill %q is a directory; an unreadable replace only displaces non-directory content", op.Slug)
			}
			oldLiveID, err = fileIDOf(info)
			if err != nil {
				return err
			}
			s.runHook(HookAfterOldLiveHash)
			if err := nameRefersTo(layout.store, op.Slug, oldLiveID); err != nil {
				return errWrap(ErrAmbiguous, "managed Skill %q changed after it was verified", op.Slug)
			}
		} else {
			live, err := openLayoutDir(layout.store, op.Slug)
			if errors.Is(err, os.ErrNotExist) {
				return ErrMissing
			}
			if err != nil {
				return err
			}
			oldLive = live
			defer oldLive.Close()
			oldLiveID, err = rootID(oldLive)
			if err != nil {
				return err
			}
			// Prove the live tree is exactly the content this operation was
			// prepared against; a stale replacement must never overwrite Store
			// edits made since.
			liveDigest, err := source.TreeDigestRoot(ctx, oldLive)
			if err != nil {
				return err
			}
			if liveDigest != op.OldDigest {
				return errWrap(ErrLiveChanged, "managed Skill %q changed since the replace was prepared (expected digest %s, observed %s)", op.Slug, op.OldDigest, liveDigest)
			}
			// Test seam: an old live tree swapped in here must fail the
			// source-name re-proof below and never move into recovery.
			s.runHook(HookAfterOldLiveHash)
			if err := nameRefersTo(layout.store, op.Slug, oldLiveID); err != nil {
				return errWrap(ErrAmbiguous, "managed Skill %q changed after it was verified", op.Slug)
			}
		}
	default:
		return fmt.Errorf("unknown operation kind %q", op.Kind)
	}
	// The Baseline candidate is copied exclusively from the digest-proven
	// staged tree before any live mutation, and every created node is
	// recorded in the invocation's manifest so a pre-mutation failure can
	// be discarded exactly. It becomes .skillctl/baselines/<skillID> at
	// finalization, after the SQLite commit fixes the Skill id. A
	// keep-baseline operation (rollback) installs no candidate: its
	// finalization retains the existing Baseline.
	var candID fileID
	if op.BaselineMode != BaselineKeep {
		if exists, err := childExists(opDir, "baseline"); err != nil {
			return err
		} else if exists {
			return errWrap(ErrAmbiguous, "a Baseline candidate already exists for operation %d", op.ID)
		}
		cand, id, err := createPinnedDirExclusive(opDir, "baseline", 0o755)
		if err != nil {
			return err
		}
		candID = id
		candRel := strconv.FormatInt(op.ID, 10) + "/baseline"
		staged.ownership.record(candRel, candID)
		if err := copyStagedTree(ctx, tree, ".", cand, candRel, &domain.Guard{AllowLarge: true}, staged.ownership); err != nil {
			cand.Close()
			return err
		}
		candDigest, err := source.TreeDigestRoot(ctx, cand)
		cand.Close()
		if err != nil {
			return err
		}
		if candDigest != op.NewDigest {
			return fmt.Errorf("Baseline candidate for operation %d does not match the staged digest", op.ID)
		}
		staged.candidateID = candID
		if err := nameRefersTo(opDir, "baseline", candID); err != nil {
			return errWrap(ErrAmbiguous, "Baseline candidate for operation %d changed after it was copied", op.ID)
		}
		if err := layout.verifyOpDir(opDir, op.ID); err != nil {
			return err
		}
	}
	// First live mutation: the proven old live tree moves into the
	// operation's recovery slot (replaces only).
	recName := strconv.FormatInt(op.ID, 10)
	var recID fileID
	if op.Kind == KindReplace {
		if exists, err := childExists(layout.recovery, recName); err != nil {
			return err
		} else if exists {
			return errWrap(ErrAmbiguous, "recovery slot for operation %d already exists", op.ID)
		}
		if op.oldUnreadable() {
			if err := moveNonTree(layout, layout.store, op.Slug, layout.recovery, recName, oldLiveID); err != nil {
				if errors.Is(err, errDestinationExists) {
					return errWrap(ErrAmbiguous, "recovery slot for operation %d appeared during replacement", op.ID)
				}
				return err
			}
			recID = oldLiveID
		} else {
			recID, err = movePinned(ctx, layout, layout.store, op.Slug, layout.recovery, recName, oldLiveID, op.OldDigest)
			if err != nil {
				if errors.Is(err, errDestinationExists) {
					return errWrap(ErrAmbiguous, "recovery slot for operation %d appeared during replacement", op.ID)
				}
				return err
			}
		}
		s.runHook(HookAfterOldLiveMoved)
	}
	// Second live mutation: the staged tree becomes the live tree with an
	// atomic no-replace rename; movePinned re-proves the installed tree
	// from its destination handle against the staged identity retained
	// above, so a byte-identical foreign destination is preserved.
	liveID, err := movePinned(ctx, layout, opDir, "tree", layout.store, op.Slug, treeID, op.NewDigest)
	if err != nil {
		if errors.Is(err, errDestinationExists) && op.Kind == KindImport {
			return ErrUnmanaged
		}
		if errors.Is(err, errDestinationExists) {
			// A foreign live tree appeared: the old content stays in the
			// recovery slot and every candidate is preserved.
			return errWrap(ErrAmbiguous, "live Skill %q appeared during install; the recovery slot and staging are preserved", op.Slug)
		}
		if op.Kind == KindReplace {
			// Restore the old content best-effort; on failure the recovery
			// slot and the pending journal preserve it for recovery. The
			// move re-proves the recovery object against the identity
			// retained from the live→recovery move.
			if _, statErr := layout.store.Lstat(op.Slug); os.IsNotExist(statErr) {
				if op.oldUnreadable() {
					if err := moveNonTree(layout, layout.recovery, recName, layout.store, op.Slug, recID); err != nil {
						return errWrap(ErrAmbiguous, "operation %d could not restore its old content: %v", op.ID, err)
					}
				} else if _, mvErr := movePinned(ctx, layout, layout.recovery, recName, layout.store, op.Slug, recID, op.OldDigest); mvErr != nil {
					return errWrap(ErrAmbiguous, "operation %d could not restore its old content: %v", op.ID, mvErr)
				}
			}
		}
		return err
	}
	// The durable install proof is written only after every physical move
	// completed; the identities come from the retained handles and the move
	// returns. A proof write failure is conservative: the moved state
	// cannot be attributed to the operation.
	if err := writeProof(opDir, installProof{
		opID: op.ID, kind: op.Kind, slug: op.Slug,
		oldDigest: op.OldDigest, newDigest: op.NewDigest,
		opDirID: opDirID, liveID: liveID, candidateID: candID, recoveryID: recID,
	}); err != nil {
		return errWrap(ErrAmbiguous, "install proof of operation %d could not be written after the live transition: %v", op.ID, err)
	}
	// A public Store method reports success only after the logical layout
	// and the operation directory still identify their pinned handles.
	if err := layout.verify(); err != nil {
		return err
	}
	return layout.verifyOpDir(opDir, op.ID)
}
