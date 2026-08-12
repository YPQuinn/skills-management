package skillstore

import (
	"errors"
	"os"
	"strconv"
)

// terminalSlotPrefix and proofSlotPrefix name the deterministic cleanup
// slots of one operation: the operation directory is detached into
// staging/.skillctl-term-<opID> and the install proof into
// staging/<opID>/.skillctl-proof-<opID>. The names are deterministic so an
// interrupted cleanup always resumes at the same names, and they never
// collide with the seq-based .skill-del-N trash slots of the other
// deletion paths.
const (
	terminalSlotPrefix = ".skillctl-term-"
	proofSlotPrefix    = ".skillctl-proof-"
)

func terminalSlotName(opID int64) string { return terminalSlotPrefix + strconv.FormatInt(opID, 10) }
func proofSlotName(opID int64) string    { return proofSlotPrefix + strconv.FormatInt(opID, 10) }

// scanTerminalEvidence is the pre-mutation validation of the receipt-bound
// evidence state: the deterministic removal hook fires here, the canonical
// operation directory must carry the receipt's identity and only the
// receipt-bound proof (or its captured slot), and the canonical directory
// and the deterministic terminal slot must never coexist. The slot-only
// resume state and the fully-clean state are accepted. Nothing is mutated.
func (l *storeLayout) scanTerminalEvidence(opID int64, receipt CleanupReceipt) error {
	termSlot := terminalSlotName(opID)
	slotExists, err := childExists(l.staging, termSlot)
	if err != nil {
		return err
	}
	opDir, err := l.opDir(opID)
	switch {
	case err == nil:
		defer opDir.Close()
		if slotExists {
			return errWrap(ErrAmbiguous, "operation directory %d and its terminal slot coexist; all evidence is preserved", opID)
		}
		if err := l.verifyOpDir(opDir, opID); err != nil {
			return err
		}
		id, err := rootID(opDir)
		if err != nil {
			return err
		}
		if id != receipt.opDirID {
			return errWrap(ErrAmbiguous, "operation directory %d is not the object %d:%d the receipt records", opID, receipt.opDirID.dev, receipt.opDirID.ino)
		}
		if l.hook != nil {
			l.hook(HookBeforeRemoveOp)
		}
		return l.scanTerminalChildren(opDir, opID, receipt.proofID)
	case errors.Is(err, os.ErrNotExist):
		if l.hook != nil {
			l.hook(HookBeforeRemoveOp)
		}
		return nil
	default:
		return errWrap(ErrAmbiguous, "opening the evidence of operation %d: %v", opID, err)
	}
}

// scanTerminalChildren strictly scans one evidence directory: it may
// contain only the receipt-bound install proof or the deterministic slot
// capturing it; any other child is foreign and preserved with ErrAmbiguous.
func (l *storeLayout) scanTerminalChildren(opDir *os.Root, opID int64, proofID fileID) error {
	dir, err := opDir.Open(".")
	if err != nil {
		return errWrap(ErrAmbiguous, "operation directory %d cannot be listed: %v", opID, err)
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return errWrap(ErrAmbiguous, "operation directory %d cannot be listed: %v", opID, err)
	}
	proofSlot := proofSlotName(opID)
	var proofPresent, slotPresent bool
	for _, e := range entries {
		switch e.Name() {
		case "proof":
			proofPresent = true
		case proofSlot:
			slotPresent = true
		default:
			return errWrap(ErrAmbiguous, "operation directory %d contains foreign child %q; it is preserved", opID, e.Name())
		}
	}
	if proofPresent && slotPresent {
		return errWrap(ErrAmbiguous, "operation directory %d contains both its proof and its captured slot; it is preserved", opID)
	}
	if slotPresent {
		info, err := opDir.Lstat(proofSlot)
		if err != nil {
			return errWrap(ErrAmbiguous, "operation directory %d captured slot cannot be identified: %v", opID, err)
		}
		id, err := fileIDOf(info)
		if err != nil {
			return errWrap(ErrAmbiguous, "operation directory %d captured slot cannot be identified: %v", opID, err)
		}
		if id != proofID {
			return errWrap(ErrAmbiguous, "operation directory %d captured slot is not the receipt-bound proof; it is preserved", opID)
		}
	}
	return nil
}

// removeTerminalEvidence executes the receipt-bound evidence removal state
// machine. Every partial state is resumable: the canonical operation
// directory (with its proof, with only the captured-proof slot, or empty)
// is drained and detached into the deterministic terminal slot; the
// terminal slot alone is unlinked; both absent means the cleanup already
// completed. Only the re-verified captured objects are ever unlinked, and
// the captured operation directory must be empty before its unlink, so a
// foreign child captured with it is preserved. The deterministic removal
// hook runs after the removal and the final absence of both evidence forms
// is re-checked before success.
func (l *storeLayout) removeTerminalEvidence(opID int64, receipt CleanupReceipt) error {
	name := strconv.FormatInt(opID, 10)
	termSlot := terminalSlotName(opID)
	opDir, err := l.opDir(opID)
	switch {
	case err == nil:
		defer opDir.Close()
		id, err := rootID(opDir)
		if err != nil {
			return errWrap(ErrAmbiguous, "operation directory %d cannot be identified: %v", opID, err)
		}
		if id != receipt.opDirID {
			return errWrap(ErrAmbiguous, "operation directory %d is not the object %d:%d the receipt records", opID, receipt.opDirID.dev, receipt.opDirID.ino)
		}
		if err := l.scanTerminalChildren(opDir, opID, receipt.proofID); err != nil {
			return err
		}
		if err := l.removeProofFromEvidence(opDir, opID, receipt.proofID); err != nil {
			return err
		}
		if err := detachTerminalDir(l.staging, name, termSlot, receipt.opDirID); err != nil {
			return err
		}
	case errors.Is(err, os.ErrNotExist):
		slotExists, err := childExists(l.staging, termSlot)
		if err != nil {
			return err
		}
		if slotExists {
			if err := removeTerminalDir(l.staging, termSlot, receipt.opDirID); err != nil {
				return err
			}
		}
	default:
		return errWrap(ErrAmbiguous, "opening the evidence of operation %d: %v", opID, err)
	}
	if l.hook != nil {
		l.hook(HookAfterRemoveOp)
	}
	for _, n := range []string{name, termSlot} {
		if exists, err := childExists(l.staging, n); err != nil {
			return err
		} else if exists {
			return errWrap(ErrAmbiguous, "operation evidence %q reappeared after its removal", n)
		}
	}
	return nil
}

// removeProofFromEvidence removes the receipt-bound install proof (or its
// captured slot) from the evidence directory, so a foreign child can never
// be detached together with the operation directory.
func (l *storeLayout) removeProofFromEvidence(opDir *os.Root, opID int64, proofID fileID) error {
	proofSlot := proofSlotName(opID)
	hasProof, err := childExists(opDir, "proof")
	if err != nil {
		return err
	}
	if hasProof {
		return detachTerminalFile(opDir, "proof", proofSlot, proofID)
	}
	hasSlot, err := childExists(opDir, proofSlot)
	if err != nil {
		return err
	}
	if hasSlot {
		return removeCapturedFile(opDir, proofSlot, proofID)
	}
	return nil
}

// detachTerminalFile detaches the single-component proof into its
// deterministic slot and unlinks only the re-verified captured object. The
// deterministic test seam runs after the detach and re-verification,
// immediately before the slot is unlinked, exactly like deleteVerified's
// trash-slot seam.
func detachTerminalFile(parent *os.Root, name, slotName string, wantID fileID) error {
	if err := nameRefersTo(parent, name, wantID); err != nil {
		return errWrap(ErrAmbiguous, "%s no longer refers to the verified proof: %v", name, err)
	}
	if err := renameNoReplaceAt(parent, name, parent, slotName); err != nil {
		if errors.Is(err, errDestinationExists) {
			return errWrap(ErrAmbiguous, "proof slot %q already exists; nothing was removed", slotName)
		}
		return errWrap(ErrAmbiguous, "%s could not be detached for its verified removal: %v", name, err)
	}
	if err := syncRoot(parent); err != nil {
		return errWrap(ErrAmbiguous, "%s was detached to %q but its parent could not be synced: %v", name, slotName, err)
	}
	if err := verifyTrashObject(parent, slotName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "detached proof %q cannot be proven as the verified object %d:%d; it is preserved: %v",
			name, wantID.dev, wantID.ino, err)
	}
	if testBeforeDeleteUnlinkHook != nil {
		testBeforeDeleteUnlinkHook(parent, name, slotName)
	}
	if err := nameRefersTo(parent, slotName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "proof slot %q changed before the verified removal; it is preserved", slotName)
	}
	return parent.Remove(slotName)
}

// removeCapturedFile unlinks the slot that already captured the proof,
// after re-verifying that the slot still refers to the receipt-bound
// object. The unlink itself is the same final-unlink boundary deleteVerified
// documents for its trash slot.
func removeCapturedFile(parent *os.Root, slotName string, wantID fileID) error {
	if err := nameRefersTo(parent, slotName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "proof slot %q no longer refers to the verified proof: %v", slotName, err)
	}
	info, err := parent.Lstat(slotName)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errWrap(ErrAmbiguous, "proof slot %q is not a regular file; it is preserved", slotName)
	}
	return parent.Remove(slotName)
}

// detachTerminalDir detaches the (empty) operation directory into its
// deterministic terminal slot and unlinks only the re-verified empty
// captured object.
func detachTerminalDir(parent *os.Root, name, slotName string, wantID fileID) error {
	if err := nameRefersTo(parent, name, wantID); err != nil {
		return errWrap(ErrAmbiguous, "%s no longer refers to the verified operation directory: %v", name, err)
	}
	if err := renameNoReplaceAt(parent, name, parent, slotName); err != nil {
		if errors.Is(err, errDestinationExists) {
			return errWrap(ErrAmbiguous, "terminal slot %q already exists; nothing was removed", slotName)
		}
		return errWrap(ErrAmbiguous, "%s could not be detached for its verified removal: %v", name, err)
	}
	if err := syncRoot(parent); err != nil {
		return errWrap(ErrAmbiguous, "%s was detached to %q but its parent could not be synced: %v", name, slotName, err)
	}
	return removeTerminalDir(parent, slotName, wantID)
}

// removeTerminalDir unlinks the deterministic terminal slot after
// re-verifying that it still refers to the receipt-bound operation
// directory and that the captured directory is empty. A non-empty capture
// means a foreign child appeared; the slot is preserved with ErrAmbiguous.
// The final unlink carries the same residual race deleteVerified documents
// for its trash slot.
func removeTerminalDir(parent *os.Root, slotName string, wantID fileID) error {
	if err := nameRefersTo(parent, slotName, wantID); err != nil {
		return errWrap(ErrAmbiguous, "terminal slot %q no longer refers to the verified operation directory: %v", slotName, err)
	}
	dir, err := openLayoutDir(parent, slotName)
	if err != nil {
		return errWrap(ErrAmbiguous, "terminal slot %q cannot be pinned: %v", slotName, err)
	}
	id, err := rootID(dir)
	if err != nil {
		dir.Close()
		return errWrap(ErrAmbiguous, "terminal slot %q cannot be identified: %v", slotName, err)
	}
	if id != wantID {
		dir.Close()
		return errWrap(ErrAmbiguous, "terminal slot %q is not the captured operation directory; it is preserved", slotName)
	}
	f, err := dir.Open(".")
	if err != nil {
		dir.Close()
		return errWrap(ErrAmbiguous, "terminal slot %q cannot be listed: %v", slotName, err)
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	dir.Close()
	if err != nil {
		return errWrap(ErrAmbiguous, "terminal slot %q cannot be listed: %v", slotName, err)
	}
	if len(entries) > 0 {
		return errWrap(ErrAmbiguous, "terminal slot %q captured a non-empty directory; it is preserved", slotName)
	}
	return parent.Remove(slotName)
}
