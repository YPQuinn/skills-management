package skillstore

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// receiptAction names the terminal Store action a CleanupReceipt
// authorizes. The action is part of the fixed receipt format, never
// inferred from the row alone.
type receiptAction string

const (
	actionFinalize receiptAction = "finalize"
	actionRestore  receiptAction = "restore"
)

// receiptVersion is the fixed format version of the terminal receipt.
const receiptVersion = 1

// receiptLineCount is the fixed number of lines in the receipt format.
const receiptLineCount = 13

// CleanupReceipt is the opaque durable authorization for terminal evidence
// cleanup. It is a fixed, strictly parsed record owned by skillstore: the
// format version and terminal action, the full operation identity, the
// physical identities of the operation directory and its install proof,
// and the terminal-result expectations that reject a byte-identical
// foreign replacement. For a finalize receipt the expectations are the
// installed live tree, the installed Baseline, and (for a replace) the
// rotated previous snapshot; for a restore receipt they are the recovered
// live tree (or its required absence for an import). The application and
// the state layer persist and pass the receipt bytes without interpreting
// them: ParseCleanupReceipt is the only way to turn bytes back into the
// opaque value, and Bytes is the only way to persist it.
type CleanupReceipt struct {
	action  receiptAction
	op      Operation // identity fields only: ID, SkillID, Slug, Kind, OldDigest, NewDigest
	opDirID fileID
	proofID fileID
	liveID  fileID // finalize: installed live; restore: recovered live (zero = must be absent)
	baseID  fileID // finalize: installed Baseline; restore: zero
	prevID  fileID // finalize replace: rotated previous; restore: zero
}

// Empty reports whether preparation found no operation evidence at all
// (the never-started or fully-cleaned import restore). Such a restore is
// already complete: the caller clears the intent directly without
// persisting a receipt or running a cleanup.
func (r CleanupReceipt) Empty() bool { return r.action == "" }

// Bytes returns the canonical fixed encoding of the receipt. The encoding
// is deterministic: a parsed receipt re-encodes to the exact bytes it was
// parsed from, so the persisted row and the receipt in memory can be
// compared byte-for-byte by the CAS deletion.
func (r CleanupReceipt) Bytes() []byte {
	return []byte(fmt.Sprintf("%d\n%s\n%d\n%d\n%s\n%s\n%s\n%s\n%d %d\n%d %d\n%d %d\n%d %d\n%d %d\n",
		receiptVersion, r.action,
		r.op.ID, r.op.SkillID, r.op.Slug, r.op.Kind, r.op.OldDigest, r.op.NewDigest,
		r.opDirID.dev, r.opDirID.ino,
		r.proofID.dev, r.proofID.ino,
		r.liveID.dev, r.liveID.ino,
		r.baseID.dev, r.baseID.ino,
		r.prevID.dev, r.prevID.ino))
}

// ParseCleanupReceipt strictly parses the fixed receipt encoding. A
// malformed, foreign, or versioned-otherwise record is rejected: the
// action, the operation identity fields, and the terminal expectations
// must satisfy the format invariants of the action, so a receipt can never
// silently authorize a different cleanup than the one it was issued for.
func ParseCleanupReceipt(data []byte) (CleanupReceipt, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != receiptLineCount {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt has %d fields, want %d", len(lines), receiptLineCount)
	}
	if lines[0] != strconv.Itoa(receiptVersion) {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt version %q is not supported", lines[0])
	}
	var r CleanupReceipt
	switch receiptAction(lines[1]) {
	case actionFinalize:
		r.action = actionFinalize
	case actionRestore:
		r.action = actionRestore
	default:
		return CleanupReceipt{}, fmt.Errorf("terminal receipt action %q is unknown", lines[1])
	}
	var err error
	if r.op.ID, err = strconv.ParseInt(lines[2], 10, 64); err != nil {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt operation id: %v", err)
	}
	if r.op.SkillID, err = strconv.ParseInt(lines[3], 10, 64); err != nil {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt Skill id: %v", err)
	}
	r.op.Slug = lines[4]
	r.op.Kind = lines[5]
	r.op.OldDigest = lines[6]
	r.op.NewDigest = lines[7]
	if r.op.ID <= 0 || r.op.Slug == "" || r.op.NewDigest == "" {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt identity is incomplete")
	}
	if r.op.Kind != KindImport && r.op.Kind != KindReplace && r.op.Kind != KindBaseline {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt kind %q is unknown", r.op.Kind)
	}
	var ids [5]fileID
	for i := 0; i < 5; i++ {
		parts := strings.Fields(lines[8+i])
		if len(parts) != 2 {
			return CleanupReceipt{}, fmt.Errorf("terminal receipt: malformed identity %q", lines[8+i])
		}
		dev, derr := strconv.ParseUint(parts[0], 10, 64)
		ino, ierr := strconv.ParseUint(parts[1], 10, 64)
		if derr != nil || ierr != nil {
			return CleanupReceipt{}, fmt.Errorf("terminal receipt: malformed identity %q", lines[8+i])
		}
		ids[i] = fileID{dev: dev, ino: ino}
	}
	r.opDirID, r.proofID, r.liveID, r.baseID, r.prevID = ids[0], ids[1], ids[2], ids[3], ids[4]
	if r.opDirID == (fileID{}) || r.proofID == (fileID{}) {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt must bind the operation directory and its proof")
	}
	switch r.action {
	case actionFinalize:
		if r.op.SkillID <= 0 {
			return CleanupReceipt{}, fmt.Errorf("finalize receipt must bind the committed Skill id")
		}
		if r.liveID == (fileID{}) || r.baseID == (fileID{}) {
			return CleanupReceipt{}, fmt.Errorf("finalize receipt must bind the installed live and Baseline identities")
		}
		if r.op.Kind == KindImport && r.prevID != (fileID{}) {
			return CleanupReceipt{}, fmt.Errorf("an import finalize receipt may not bind a previous identity")
		}
		if r.op.Kind == KindReplace && r.prevID == (fileID{}) {
			return CleanupReceipt{}, fmt.Errorf("a replace finalize receipt must bind the rotated previous identity")
		}
	case actionRestore:
		if r.baseID != (fileID{}) || r.prevID != (fileID{}) {
			return CleanupReceipt{}, fmt.Errorf("a restore receipt may not bind Baseline or previous identities")
		}
		if r.op.Kind == KindImport && r.liveID != (fileID{}) {
			return CleanupReceipt{}, fmt.Errorf("an import restore receipt requires the live tree to be absent")
		}
		if r.op.Kind == KindReplace && r.liveID == (fileID{}) {
			return CleanupReceipt{}, fmt.Errorf("a replace restore receipt must bind the recovered live identity")
		}
	}
	// The persisted encoding must be exactly the canonical Bytes() form:
	// a payload that merely parses to the same fields (missing final
	// newline, extra or non-single whitespace, leading-zero numbers) is
	// rejected, so the row can only ever carry the bytes the CAS deletion
	// compares and the receipt re-encodes identically.
	if canonical := r.Bytes(); !bytes.Equal(canonical, data) {
		return CleanupReceipt{}, fmt.Errorf("terminal receipt is not in its canonical encoding")
	}
	return r, nil
}

// validateReceiptBinding requires the receipt's recorded operation identity
// to equal the operation being cleaned, so a stale or foreign row can
// never be cleaned through another operation's receipt.
func validateReceiptBinding(r CleanupReceipt, op Operation) error {
	switch {
	case r.op.ID != op.ID:
		return fmt.Errorf("receipt belongs to operation %d, not %d", r.op.ID, op.ID)
	case r.op.SkillID != op.SkillID:
		return fmt.Errorf("receipt Skill id %d does not match operation %d", r.op.SkillID, op.ID)
	case r.op.Slug != op.Slug:
		return fmt.Errorf("receipt slug %q does not match operation %d", r.op.Slug, op.ID)
	case r.op.Kind != op.Kind:
		return fmt.Errorf("receipt kind %q does not match operation %d", r.op.Kind, op.ID)
	case r.op.OldDigest != op.OldDigest:
		return fmt.Errorf("receipt old digest does not match operation %d", op.ID)
	case r.op.NewDigest != op.NewDigest:
		return fmt.Errorf("receipt new digest does not match operation %d", op.ID)
	}
	return nil
}

// newFinalizeReceipt builds the terminal finalize receipt from the
// validated install proof: the terminal expectations are the installed
// live tree, the Baseline (the installed candidate for an advancing
// operation, or the retained pre-operation Baseline sampled by the caller
// for a keep-baseline rollback), and (for a replace) the rotated previous
// snapshot, all by their physical identities.
func newFinalizeReceipt(op Operation, opDirID, proofID fileID, proof *installProof, baseID fileID) CleanupReceipt {
	return CleanupReceipt{
		action: actionFinalize, op: op,
		opDirID: opDirID, proofID: proofID,
		liveID: proof.liveID, baseID: baseID, prevID: proof.recoveryID,
	}
}

// newRestoreReceipt builds the terminal restore receipt from the validated
// install proof: for a replace the terminal expectation is the recovered
// live tree (the object the proof moved into recovery); for an import the
// live tree must be absent after the restore.
func newRestoreReceipt(op Operation, opDirID, proofID fileID, proof *installProof) CleanupReceipt {
	return CleanupReceipt{
		action: actionRestore, op: op,
		opDirID: opDirID, proofID: proofID,
		liveID: proof.recoveryID,
	}
}

// errReceiptAmbiguous wraps a receipt-format or binding failure as
// ErrAmbiguous with the preserved-candidates contract.
func errReceiptAmbiguous(format string, args ...any) error {
	return errWrap(ErrAmbiguous, format, args...)
}
