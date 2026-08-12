package skillstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// witnessAction names the finalize transient slot a witness records. The
// action is part of the fixed canonical format, never inferred from the
// file name alone.
type witnessAction string

const (
	witnessBaseline  witnessAction = "baseline-old"
	witnessStalePrev witnessAction = "stale-previous"
)

// witnessVersion and witnessLineCount fix the canonical witness format.
const (
	witnessVersion   = 1
	witnessLineCount = 6
)

// transientWitness is the durable progress witness of one finalize
// transient: it binds the operation, the transient slot, the witness
// file's own physical identity, the moved transient's physical identity,
// and the transient's expected digest. It is written before the transient
// moves and removed after the transient is drained, so a crash between the
// move and the drain leaves the fresh process able to attribute the
// transient to this operation — and any other content in the transient
// slot, at the witness name, or at the witness tombstone is foreign and
// preserved. The witness is the narrowest progress record: it binds
// exactly one transient object, never a tree manifest.
type transientWitness struct {
	opID   int64
	action witnessAction
	selfID fileID
	id     fileID
	digest string
}

// witnessTombName is the deterministic no-replace tombstone of one witness
// file: the removal detaches the canonical name into the tombstone and
// unlinks only the re-verified captured object, so an interrupted removal
// resumes on the next preparation by the tombstone's own identity.
func witnessTombName(name string) string { return name + ".del" }

// witnessFilePolicy conservatively refuses anything that cannot be proven
// as the exclusive plain file this operation wrote: a symlink, a special
// node, or a hardlinked witness (nlink > 1) is never accepted or removed.
func witnessFilePolicy(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("is a symlink")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("is not a regular file")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Nlink != 1 {
		return fmt.Errorf("has %d links; a hardlinked witness cannot be proven exclusive", st.Nlink)
	}
	return nil
}

// bytes returns the canonical fixed encoding of the witness. The encoding
// is deterministic: a parsed witness re-encodes to the exact bytes it was
// parsed from, so a persisted witness can be compared byte-for-byte.
func (w *transientWitness) bytes() []byte {
	return []byte(fmt.Sprintf("%d\n%d\n%s\n%d %d\n%d %d\n%s\n",
		witnessVersion, w.opID, w.action,
		w.selfID.dev, w.selfID.ino,
		w.id.dev, w.id.ino, w.digest))
}

// parseTransientWitness strictly parses the canonical witness encoding and
// requires the exact canonical round trip: a payload that merely parses to
// the same fields (missing final newline, extra whitespace, leading-zero
// numbers, or a different action) is rejected.
func parseTransientWitness(data []byte) (*transientWitness, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != witnessLineCount {
		return nil, fmt.Errorf("transient witness has %d fields, want %d", len(lines), witnessLineCount)
	}
	if lines[0] != strconv.Itoa(witnessVersion) {
		return nil, fmt.Errorf("transient witness version %q is not supported", lines[0])
	}
	opID, err := strconv.ParseInt(lines[1], 10, 64)
	if err != nil || opID <= 0 {
		return nil, fmt.Errorf("transient witness has no valid operation id")
	}
	action := witnessAction(lines[2])
	if action != witnessBaseline && action != witnessStalePrev {
		return nil, fmt.Errorf("transient witness action %q is unknown", lines[2])
	}
	var ids [2]fileID
	for i := 0; i < 2; i++ {
		parts := strings.Fields(lines[3+i])
		if len(parts) != 2 {
			return nil, fmt.Errorf("transient witness has a malformed identity %q", lines[3+i])
		}
		dev, derr := strconv.ParseUint(parts[0], 10, 64)
		ino, ierr := strconv.ParseUint(parts[1], 10, 64)
		if derr != nil || ierr != nil {
			return nil, fmt.Errorf("transient witness has a malformed identity %q", lines[3+i])
		}
		ids[i] = fileID{dev: dev, ino: ino}
	}
	if lines[5] == "" {
		return nil, fmt.Errorf("transient witness has no digest")
	}
	w := &transientWitness{opID: opID, action: action, selfID: ids[0], id: ids[1], digest: lines[5]}
	if w.selfID == (fileID{}) || w.id == (fileID{}) {
		return nil, fmt.Errorf("transient witness must bind both its own and the transient identity")
	}
	if canonical := w.bytes(); !bytes.Equal(canonical, data) {
		return nil, fmt.Errorf("transient witness is not in its canonical encoding")
	}
	return w, nil
}

// writeTransientWitness durably records the witness inside the pinned
// operation directory: the file is created exclusively and no-follow, the
// witness self identity comes from the same open handle, the canonical
// bytes are written and synced, the parent is synced, and the logical name
// is re-proven against the created object, so a crash cannot lose the
// record after the transient moved and a pre-existing witness is never
// overwritten. The recorded witness is returned.
func writeTransientWitness(parent *os.Root, name string, opID int64, action witnessAction, transientID fileID, digest string) (*transientWitness, error) {
	f, err := parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return nil, err
	}
	selfID, err := fileIDFromOpen(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	w := &transientWitness{opID: opID, action: action, selfID: selfID, id: transientID, digest: digest}
	if _, err := f.Write(w.bytes()); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, err
	}
	if err := witnessFilePolicy(info); err != nil {
		return nil, err
	}
	if err := nameRefersTo(parent, name, selfID); err != nil {
		return nil, fmt.Errorf("transient witness %q is not the object that was created: %v", name, err)
	}
	dir, err := parent.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return nil, err
	}
	return w, nil
}

// readTransientWitness returns the witness at parent/name, or nil when it
// is absent. The entry must be a plain single-link regular file, the
// opened handle must be the very object the entry names, the name must
// still identify the handle after the read, the record must be strictly
// canonical, and the recorded operation, action, and witness self identity
// must match the caller's expectation. Anything else is foreign and
// reported as an error.
func readTransientWitness(parent *os.Root, name string, opID int64, action witnessAction) (*transientWitness, error) {
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := witnessFilePolicy(info); err != nil {
		return nil, fmt.Errorf("transient witness %q %v", name, err)
	}
	f, err := parent.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	fid, err := fileIDFromOpen(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	if !sameIdentity(info, fid) {
		f.Close()
		return nil, fmt.Errorf("transient witness %q changed while it was being opened", name)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	if err := nameRefersTo(parent, name, fid); err != nil {
		return nil, fmt.Errorf("transient witness %q changed while it was being read: %v", name, err)
	}
	w, err := parseTransientWitness(data)
	if err != nil {
		return nil, fmt.Errorf("transient witness %q is malformed: %v", name, err)
	}
	if w.opID != opID || w.action != action {
		return nil, fmt.Errorf("transient witness %q belongs to operation %d slot %q, not %d slot %q",
			name, w.opID, w.action, opID, action)
	}
	if w.selfID != fid {
		return nil, fmt.Errorf("transient witness %q self identity does not match the file it was read from", name)
	}
	return w, nil
}

// resolveWitnessState resolves the durable witness state at the start of a
// preparation: an interrupted removal (canonical name gone, tombstone
// present) is resumed first and only when the tombstone is the very
// witness this operation wrote; a canonical witness and its tombstone
// coexisting is foreign and preserved with ErrAmbiguous; then the
// canonical witness (or nil) is returned.
func resolveWitnessState(parent *os.Root, name string, opID int64, action witnessAction) (*transientWitness, error) {
	tomb := witnessTombName(name)
	tombExists, err := childExists(parent, tomb)
	if err != nil {
		return nil, err
	}
	if tombExists {
		canonicalExists, err := childExists(parent, name)
		if err != nil {
			return nil, err
		}
		if canonicalExists {
			return nil, errWrap(ErrAmbiguous, "witness %q and its tombstone coexist; all candidates are preserved", name)
		}
		if err := resumeWitnessRemoval(parent, name, opID, action, fileID{}); err != nil {
			return nil, err
		}
	}
	return readTransientWitness(parent, name, opID, action)
}

// verifyWitnessTree requires the transient tree at parent/name to be the
// exact object the witness recorded: identity and canonical digest sampled
// from the same opened handle. A byte-identical foreign tree, a partially
// drained tree, or an edited tree is foreign and preserved.
func verifyWitnessTree(ctx context.Context, parent *os.Root, name string, w *transientWitness) error {
	root, id, digest, err := openTreeDigest(ctx, parent, name)
	if err != nil {
		return err
	}
	root.Close()
	if id != w.id || digest != w.digest {
		return fmt.Errorf("transient %q is not the object %d:%d the witness records", name, w.id.dev, w.id.ino)
	}
	return nil
}
