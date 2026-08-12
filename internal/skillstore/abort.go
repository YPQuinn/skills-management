package skillstore

import (
	"context"
	"errors"
	"os"
	"strconv"

	"skillctl/internal/domain"
)

// Abort completes a pre-install abort: the operation never mutated the live
// path (Stage failed, or Install refused with ErrUnmanaged or ErrMissing),
// so recovery removes only the operation's own staging and never reads,
// moves, or deletes live content. The immediate abort path discards the
// staging with the identity Stage retained (DiscardStaging); recovery only
// ever sees the resulting absent state, which is safe to clear. A staging
// directory that still exists on recovery has no persisted physical proof
// (Stage-A writes no durable receipt), so even a byte-identical candidate
// is preserved with ErrAmbiguous. Contradictory evidence that a pre-install
// refusal can never create — a recovery slot, a quarantine, an install
// proof, or a deterministic terminal tombstone (.skillctl-term-<opID>) —
// is preserved and reported as ErrAmbiguous. When the full layout does not
// open, the absence is verified granularly: no Store root means nothing can
// exist, but a partial internal layout can still hold operation evidence
// and is never cleared. The application clears the operation intent only
// after Abort returns, so a failed intent deletion can be retried on the
// next write.
func (s Store) Abort(ctx context.Context, op Operation) error {
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return err
	}
	if op.Kind != KindImport && op.Kind != KindReplace {
		return errWrap(ErrAmbiguous, "unknown operation kind %q", op.Kind)
	}
	layout, err := s.openLayout()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return errWrap(ErrAmbiguous, "opening the Store layout: %v", err)
		}
		return s.abortWithoutLayout(op)
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return err
	}
	if exists, err := childExists(layout.recovery, strconv.FormatInt(op.ID, 10)); err != nil {
		return err
	} else if exists {
		return errWrap(ErrAmbiguous, "aborted operation %d has a recovery slot", op.ID)
	}
	// The deterministic terminal tombstone is terminal-cleanup evidence a
	// pre-install refusal can never create: any present object at the
	// tombstone name, regardless of bytes or type, is preserved and blocks
	// the abort.
	if exists, err := childExists(layout.staging, terminalSlotName(op.ID)); err != nil {
		return err
	} else if exists {
		return errWrap(ErrAmbiguous, "aborted operation %d has a terminal tombstone; it is preserved", op.ID)
	}
	opDir, err := layout.opDir(op.ID)
	if errors.Is(err, os.ErrNotExist) {
		// The staging was already provably discarded by the immediate
		// abort path; the absent state is safe to clear.
		return nil
	}
	if err != nil {
		return errWrap(ErrAmbiguous, "opening the staging of aborted operation %d: %v", op.ID, err)
	}
	opDir.Close()
	// A present operation directory for an aborted intent has no persisted
	// physical proof on restart: the current sample cannot prove the
	// directory is the operation's own staging (a byte-identical foreign
	// directory is indistinguishable). The aborted state is cleanable only
	// when the staging is absent; anything present is preserved.
	return errWrap(ErrAmbiguous, "aborted operation %d has surviving staging with no persisted physical proof; it is preserved", op.ID)
}

// abortWithoutLayout verifies the operation-evidence absence when the full
// internal layout cannot open: a missing Store root (or a root without any
// .skillctl) means no operation artifact can exist and the abort is
// complete; a partial internal layout is pinned component by component and
// refuses when the operation's recovery slot, staging, or deterministic
// terminal tombstone exists anywhere in it, so foreign evidence is
// preserved with ErrAmbiguous.
func (s Store) abortWithoutLayout(op Operation) error {
	root, err := os.OpenRoot(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errWrap(ErrAmbiguous, "opening the Store: %v", err)
	}
	defer root.Close()
	internal, err := openLayoutDir(root, ".skillctl")
	if errors.Is(err, os.ErrNotExist) {
		// No internal layout at all: no operation evidence can exist.
		return nil
	}
	if err != nil {
		return errWrap(ErrAmbiguous, "opening the internal layout: %v", err)
	}
	defer internal.Close()
	for _, name := range []string{"recovery", "staging"} {
		dir, derr := openLayoutDir(internal, name)
		if errors.Is(derr, os.ErrNotExist) {
			continue
		}
		if derr != nil {
			return errWrap(ErrAmbiguous, "opening the %s layout: %v", name, derr)
		}
		exists, cerr := childExists(dir, strconv.FormatInt(op.ID, 10))
		if cerr != nil {
			dir.Close()
			return cerr
		}
		if exists {
			dir.Close()
			return errWrap(ErrAmbiguous, "aborted operation %d has a %s slot", op.ID, name)
		}
		if name == "staging" {
			tomb, terr := childExists(dir, terminalSlotName(op.ID))
			if terr != nil {
				dir.Close()
				return terr
			}
			if tomb {
				dir.Close()
				return errWrap(ErrAmbiguous, "aborted operation %d has a terminal tombstone; it is preserved", op.ID)
			}
		}
		dir.Close()
	}
	return nil
}
