package distribution

import (
	"errors"
	"fmt"
	"os"
)

// CreateLink samples a symlink inside a fresh private directory before
// publishing it with a no-overwrite rename. The caller persists isolation
// before calling, and persist must durably record the proof before returning.
// The caller cleans staging with DiscardCreateStaging, except on
// ErrSlotOccupied (that directory is not ours). A crash leaves the recorded
// slot for recovery, never a guessed live proof.
func CreateLink(containerPath, slug, rawTarget, isolation string, persist func(LinkProof) error) (LinkProof, error) {
	parent, err := openContainerHandle(containerPath, true)
	if err != nil {
		return LinkProof{}, err
	}
	defer parent.Close()
	iso, err := parent.mkdirIsolation(isolation)
	if err != nil {
		return LinkProof{}, err
	}
	defer iso.Close()
	if err := iso.symlink(rawTarget, IsolatedEntry); err != nil {
		return LinkProof{}, fmt.Errorf("creating the staged link: %w", err)
	}
	proof, err := iso.probeStableSymlink(IsolatedEntry)
	if err != nil {
		return LinkProof{}, fmt.Errorf("sampling the staged link: %w", err)
	}
	if proof.Raw != rawTarget {
		return LinkProof{}, ErrLinkMismatch
	}
	if err := persist(proof); err != nil {
		return proof, err
	}
	got, err := iso.probeStableSymlink(IsolatedEntry)
	if err != nil || !proof.Matches(got) {
		return proof, ErrLinkMismatch
	}
	if err := iso.renameTo(IsolatedEntry, parent, slug); err != nil {
		if errors.Is(err, errDestinationExists) {
			return proof, ErrEntryExists
		}
		return proof, err
	}
	if testAfterCreateBeforeStat != nil {
		testAfterCreateBeforeStat()
	}
	got, err = parent.probeStableSymlink(slug)
	if err != nil || !proof.Matches(got) {
		return proof, ErrLinkMismatch
	}
	return proof, nil
}

// DiscardCreateStaging removes only the proven staged symlink and an empty
// staging directory. Unknown contents are preserved; the caller retains the
// intent on error. It never restores staging onto, or unlinks, the live slug.
func DiscardCreateStaging(containerPath, isolation string, proof LinkProof) error {
	parent, iso, err := openIsolation(containerPath, isolation)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	defer iso.Close()
	got, err := iso.probeStableSymlink(IsolatedEntry)
	if !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		if !proof.Matches(got) {
			return ErrLinkMismatch
		}
		if err := iso.unlink(IsolatedEntry); err != nil {
			return err
		}
	}
	return parent.rmdir(isolation)
}

// The hook represents an external replacement after the live name appears,
// before any post-publication observation (the original vulnerable window).
var testAfterCreateBeforeStat func()
