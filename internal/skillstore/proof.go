package skillstore

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// installProof is the fixed durable record of one completed install. It is
// written and synced only after every physical live-path move finished, so
// recovery can attribute an installed tree, a Baseline candidate, and a
// recovery slot to the exact objects this operation moved. The format is
// fixed and canonical: operation identity, kind, slug, old/new digests,
// the proof file's own physical identity (selfID), and the physical
// identities of the operation directory, the installed live tree, the
// Baseline candidate, and the recovery slot (zero for an import). The
// selfID binds the record to the exact proof file this operation created,
// so a byte-identical proof copied into a fresh inode is never accepted.
// No compatibility shim exists: a proof that does not parse exactly is
// invalid.
type installProof struct {
	opID        int64
	kind        string
	slug        string
	oldDigest   string
	newDigest   string
	selfID      fileID
	opDirID     fileID
	liveID      fileID
	candidateID fileID
	recoveryID  fileID
}

// proofLineCount is the fixed number of fields in the proof format.
const proofLineCount = 10

// bytes returns the canonical fixed encoding of the proof. The encoding is
// deterministic: a parsed proof re-encodes to the exact bytes it was
// parsed from, so the persisted record can be compared byte-for-byte.
func (p *installProof) bytes() []byte {
	return []byte(fmt.Sprintf("%d\n%s\n%s\n%s\n%s\n%d %d\n%d %d\n%d %d\n%d %d\n%d %d\n",
		p.opID, p.kind, p.slug, p.oldDigest, p.newDigest,
		p.selfID.dev, p.selfID.ino,
		p.opDirID.dev, p.opDirID.ino,
		p.liveID.dev, p.liveID.ino,
		p.candidateID.dev, p.candidateID.ino,
		p.recoveryID.dev, p.recoveryID.ino))
}

// writeProof durably records the install proof inside the pinned operation
// directory: the file is created exclusively and no-follow, the proof self
// identity comes from that same opened handle, the canonical bytes are
// written and synced, the parent is synced, and the logical name is
// re-proven against the created object, so a crash cannot lose the record
// after Install returns and a swapped or copied proof file is never
// blessed.
func writeProof(opDir *os.Root, p installProof) error {
	f, err := opDir.OpenFile("proof", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	p.selfID, err = fileIDFromOpen(f)
	if err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(p.bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	info, err := opDir.Lstat("proof")
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("install proof is not a regular file")
	}
	if err := nameRefersTo(opDir, "proof", p.selfID); err != nil {
		return fmt.Errorf("install proof is not the object that was created: %v", err)
	}
	dir, err := opDir.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// readProof returns the operation's install proof and the physical identity
// of the proof file itself, both sampled from a single no-follow open: the
// entry must be a regular file, the opened handle must be the very object
// the entry names, the name must still identify the handle after the read,
// the record must be strictly canonical, and the recorded self identity
// must equal the opened handle's identity. nil is returned when the
// operation has no proof (its install never completed). A byte-identical
// proof copied into a fresh inode, a malformed record, or a swapped name
// is rejected, so the retained proof-file identity always authorizes
// cleanup of the exact proof this operation created.
func readProof(opDir *os.Root) (*installProof, fileID, error) {
	info, err := opDir.Lstat("proof")
	if errors.Is(err, os.ErrNotExist) {
		return nil, fileID{}, nil
	}
	if err != nil {
		return nil, fileID{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, fileID{}, fmt.Errorf("install proof is not a regular file")
	}
	f, err := opDir.OpenFile("proof", os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fileID{}, err
	}
	fid, err := fileIDFromOpen(f)
	if err != nil {
		f.Close()
		return nil, fileID{}, err
	}
	if !sameIdentity(info, fid) {
		f.Close()
		return nil, fileID{}, fmt.Errorf("install proof changed while it was being opened")
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, fileID{}, err
	}
	// The logical entry must still identify the single opened object.
	if err := nameRefersTo(opDir, "proof", fid); err != nil {
		return nil, fileID{}, fmt.Errorf("install proof changed while it was being read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != proofLineCount {
		return nil, fileID{}, fmt.Errorf("install proof has %d fields, want %d", len(lines), proofLineCount)
	}
	opID, err := strconv.ParseInt(lines[0], 10, 64)
	if err != nil {
		return nil, fileID{}, fmt.Errorf("install proof: %v", err)
	}
	var ids [5]fileID
	for i := 0; i < 5; i++ {
		parts := strings.Fields(lines[5+i])
		if len(parts) != 2 {
			return nil, fileID{}, fmt.Errorf("install proof: malformed identity %q", lines[5+i])
		}
		dev, derr := strconv.ParseUint(parts[0], 10, 64)
		ino, ierr := strconv.ParseUint(parts[1], 10, 64)
		if derr != nil || ierr != nil {
			return nil, fileID{}, fmt.Errorf("install proof: malformed identity %q", lines[5+i])
		}
		ids[i] = fileID{dev: dev, ino: ino}
	}
	p := &installProof{
		opID: opID, kind: lines[1], slug: lines[2],
		oldDigest: lines[3], newDigest: lines[4],
		selfID: ids[0], opDirID: ids[1], liveID: ids[2], candidateID: ids[3], recoveryID: ids[4],
	}
	if p.selfID != fid {
		return nil, fileID{}, fmt.Errorf("install proof self identity %d:%d does not match the opened object %d:%d",
			p.selfID.dev, p.selfID.ino, fid.dev, fid.ino)
	}
	if canonical := p.bytes(); !bytes.Equal(canonical, data) {
		return nil, fileID{}, fmt.Errorf("install proof is not in its canonical encoding")
	}
	return p, fid, nil
}

// sameIdentity reports whether the entry info and the identity sampled from
// an open handle describe the same object.
func sameIdentity(info os.FileInfo, id fileID) bool {
	lid, err := fileIDOf(info)
	return err == nil && lid == id
}

// validateProof binds a durable proof to one journal operation: every field
// must match the intent, so a stale or tampered record can never authorize
// recovery of different content.
func validateProof(proof *installProof, op Operation) error {
	switch {
	case proof.opID != op.ID:
		return fmt.Errorf("proof belongs to operation %d, not %d", proof.opID, op.ID)
	case proof.kind != op.Kind:
		return fmt.Errorf("proof kind %q does not match operation kind %q", proof.kind, op.Kind)
	case proof.slug != op.Slug:
		return fmt.Errorf("proof slug %q does not match operation slug %q", proof.slug, op.Slug)
	case proof.oldDigest != op.OldDigest:
		return fmt.Errorf("proof old digest does not match the operation intent")
	case proof.newDigest != op.NewDigest:
		return fmt.Errorf("proof new digest does not match the operation intent")
	}
	return nil
}
