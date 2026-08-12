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

// StagedProof is the opaque capability Stage hands to Install and the
// application: the physical identities of the operation directory, the
// staged tree, and the Baseline candidate (bound once Install creates it),
// the canonical digest of the staged tree, and the private creation
// manifest of this Stage invocation. Every field is private; the
// application only passes the proof through, and Install consumes it for
// identity binding while a pre-install discard uses the manifest to remove
// exactly the objects this invocation created. Tests in this package may
// inspect the fields; no durable receipt is written here.
type StagedProof struct {
	opID        int64
	digest      string
	treeID      fileID
	opDirID     fileID
	candidateID fileID
	ownership   *stageOwnership
}

// Stage copies the materialized tree at srcDir into same-filesystem staging
// (.skillctl/staging/<opID>/tree), applies the per-Skill guards (file
// count, total bytes, per-file bytes; bypassed by allowLarge), re-validates
// every node by Lstat so symlinks and special nodes are rejected even with
// allowLarge, and verifies the staged tree's canonical digest against
// expectedDigest. Reads are rooted at the source tree and writes are rooted
// at the pinned staging tree with exclusive creates, so a hostile node
// planted into either tree can never be followed or escaped. The operation
// directory and every node below it are created atomically and exclusively:
// a pre-existing entry is ErrAmbiguous and preserved, never opened.
//
// Return contract: an ordinary failure (copy error, guard refusal, digest
// mismatch, source open failure) is reported only when the exact
// retained-identity cleanup of the invocation's own creation manifest
// succeeded and the operation directory is finally absent; otherwise the
// failure is reported as ErrAmbiguous with every candidate preserved. On
// ErrAmbiguous (a pre-existing or swapped node, or a cleanup that cannot be
// proven) nothing is removed and the caller must keep the operation pending
// and preserve every candidate. Before returning, the operation directory
// and the staged tree are re-proven to still identify the opened objects;
// the returned StagedProof binds Install to the exact staged object and the
// abort path to the exact objects this invocation created.
func (s Store) Stage(ctx context.Context, opID int64, srcDir, expectedDigest string, allowLarge bool) (StagedProof, error) {
	if err := s.EnsureLayout(); err != nil {
		return StagedProof{}, err
	}
	layout, err := s.openLayout()
	if err != nil {
		return StagedProof{}, err
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return StagedProof{}, err
	}
	name := strconv.FormatInt(opID, 10)
	own := newStageOwnership()
	opDir, opDirID, err := createPinnedDirExclusive(layout.staging, name, 0o755)
	if err != nil {
		return StagedProof{}, err
	}
	defer opDir.Close()
	own.record(name, opDirID)
	// Durability ordering: the operation-directory entry must be durably
	// reachable in its staging parent before Install performs any live
	// mutation. On power loss after a synced live rename but before the
	// install proof is written, recovery must still find the operation
	// directory (or fail closed on its surviving staging), never classify
	// the operation as evidence-free and clear the row while unbound live
	// content survives. The staging parent is therefore synced immediately
	// after the exclusive creation.
	if err := syncRoot(layout.staging); err != nil {
		return StagedProof{}, errWrap(ErrAmbiguous, "operation %d staging parent could not be synced after the operation directory was created: %v", opID, err)
	}
	if testAfterOpDirEntrySync != nil {
		testAfterOpDirEntrySync(layout.staging, opDirID)
	}
	// fail returns the ordinary error only when the manifest cleanup is
	// proven; an ambiguous cause and an unprovable cleanup both become
	// ErrAmbiguous with preservation.
	fail := func(cause error) (StagedProof, error) {
		if errors.Is(cause, ErrAmbiguous) {
			return StagedProof{}, cause
		}
		if err := cleanupOwnership(layout.staging, opID, own, s.hook); err != nil {
			return StagedProof{}, errWrap(ErrAmbiguous, "operation %d failed and its staging could not be provably removed: %v", opID, err)
		}
		if err := layout.verify(); err != nil {
			return StagedProof{}, errWrap(ErrAmbiguous, "operation %d failed and the layout could not be re-verified: %v", opID, err)
		}
		return StagedProof{}, cause
	}
	tree, treeID, err := createPinnedDirExclusive(opDir, "tree", 0o755)
	if err != nil {
		return fail(err)
	}
	defer tree.Close()
	own.record(name+"/tree", treeID)
	src, err := os.OpenRoot(srcDir)
	if err != nil {
		return fail(err)
	}
	defer src.Close()
	if err := copyStagedTree(ctx, src, ".", tree, name+"/tree", &domain.Guard{AllowLarge: allowLarge}, own); err != nil {
		return fail(err)
	}
	digest, err := source.TreeDigestRoot(ctx, tree)
	if err != nil {
		return fail(err)
	}
	if digest != expectedDigest {
		return fail(fmt.Errorf("staged content digest %s does not match the materialized digest %s", digest, expectedDigest))
	}
	// Test seam: a byte-identical foreign op directory or staged tree
	// swapped in here must fail the re-proofs below and be preserved.
	s.runHook(HookAfterStageHash)
	// Re-prove the logical attachment before returning: the global layout,
	// the operation directory, and the staged tree must still identify the
	// opened objects.
	if err := layout.verify(); err != nil {
		return StagedProof{}, err
	}
	if err := layout.verifyOpDir(opDir, opID); err != nil {
		return StagedProof{}, err
	}
	if err := nameRefersTo(opDir, "tree", treeID); err != nil {
		return StagedProof{}, errWrap(ErrAmbiguous, "staged tree for operation %d changed after it was verified", opID)
	}
	return StagedProof{opID: opID, digest: digest, treeID: treeID, opDirID: opDirID, ownership: own}, nil
}
