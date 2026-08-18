// Package skillstore exclusively owns the Skill Store tree: the live Skill
// directories and the internal .skillctl layout (staging, baselines,
// previous, recovery) on the same filesystem. It never executes SQL; the
// application boundary coordinates persistence and recovery through
// internal/state, and every mutation runs under the Store exclusive lock
// the application acquires. Every slug is validated against the decision-03
// grammar before it is joined into any Store path, so a crafted or
// persisted-operation slug can never escape the Store.
package skillstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"skillctl/internal/domain"
)

// Per-Skill import guards (decision 03): 10,000 files, 100 MiB total, 50
// MiB per file. allow-large bypasses these; unsafe paths and special nodes
// can never be overridden. The limits themselves live in internal/domain so
// Source materialization and Store staging enforce one shared contract.
const (
	MaxFilesPerSkill = domain.MaxFilesPerSkill
	MaxBytesPerSkill = domain.MaxBytesPerSkill
	MaxBytesPerFile  = domain.MaxBytesPerFile
)

// Operation kinds and phases recorded in the durable Store journal.
const (
	KindImport  = "import"
	KindReplace = "replace"
	// KindBaseline is the Baseline-only refresh (Keep Store and
	// independent convergence): the staged tree becomes the new Baseline
	// during finalization and the live tree is never mutated.
	KindBaseline = "baseline"
	// KindRemove is the durable Skill delete: live, Baseline, and previous
	// are isolated first; the Skill row is deleted only when the journal
	// commits; parked trees are drained only after that commit.
	KindRemove = "remove"

	// BaselineAdvance marks an ordinary replace whose finalization
	// installs the operation's Baseline candidate into .skillctl/baselines.
	BaselineAdvance = "advance"
	// BaselineKeep marks a rollback: finalization retains the existing
	// Baseline untouched, so the rolled-back Skill recomputes as
	// store_changed instead of silently re-accepting the Source.
	BaselineKeep = "keep"
	// BaselineClear marks a Baseline-only removal (conflicting Rebind):
	// the existing Baseline is isolated before the Binding/digest commit
	// and drained only after that commit.
	BaselineClear = "clear"

	PhasePending   = "pending"
	PhaseCommitted = "committed"
	// PhaseAborted is the terminal journal phase of a pre-install refusal:
	// Stage failed or Install refused before any live mutation, so recovery
	// discards only the operation's own staging and never touches live
	// content. It never carries a terminal receipt: the abort is certified
	// by the provable absence of operation evidence.
	PhaseAborted = "aborted"
	// PhaseFinalized is the terminal journal phase of a completed commit:
	// the Store performed the semantic finalization and the durable
	// terminal receipt authorizes the receipt-bound evidence cleanup.
	PhaseFinalized = "finalized"
	// PhaseRestored is the terminal journal phase of a completed restore:
	// the Store unwound the pending operation and the durable terminal
	// receipt authorizes the receipt-bound evidence cleanup.
	PhaseRestored = "restored"
)

// Sentinel errors the application maps onto stable outcomes. ErrAmbiguous
// preserves every candidate and blocks writes.
var (
	ErrUnmanaged = errors.New("a directory exists in the Skill Store that Skill Manager does not manage")
	ErrAmbiguous = errors.New("Skill Store state cannot be proven valid; all candidate content is preserved")
	ErrMissing   = errors.New("managed Skill content is missing from the Skill Store")
	// ErrLiveChanged reports that the live Skill tree no longer carries
	// the digest a replace was prepared against: a local edit landed
	// between the replace's preparation and its install, and the
	// replacement is refused instead of overwriting it.
	ErrLiveChanged = errors.New("managed Skill content changed since the replace was prepared")

	// errDestinationExists reports a no-replace rename that found its
	// destination already present.
	errDestinationExists = errors.New("destination already exists")
)

// HookPoint identifies one deterministic test seam inside a Store
// operation. The hooks are nil in production; tests use them to swap
// content at the exact windows the Stage-A invariants protect.
type HookPoint int

const (
	// HookAfterStagedHash runs after the staged tree was hashed, before
	// its source name is re-proven for the staged→live rename.
	HookAfterStagedHash HookPoint = iota
	// HookAfterOldLiveHash runs after the old live tree was hashed, before
	// it is re-proven for the live→recovery move.
	HookAfterOldLiveHash
	// HookAfterQuarantineOpen runs after the quarantined tree was opened
	// and verified against the install proof, before it is drained.
	HookAfterQuarantineOpen
	// HookAfterLayoutOpen runs after the layout handles were pinned,
	// before the public operation's first layout re-validation.
	HookAfterLayoutOpen
	// HookBeforeBaselineMoves runs after the Baseline, candidate, and
	// saved-Baseline slots were sampled for committed finalization, before
	// the first Baseline/previous source move.
	HookBeforeBaselineMoves
	// HookAfterStageHash runs after the staged tree was hashed, before
	// Stage re-proves the operation directory and the staged tree name.
	HookAfterStageHash
	// HookBeforeEnsureLayoutVerify runs before EnsureLayout's final
	// re-validation of the Store root and internal layout.
	HookBeforeEnsureLayoutVerify
	// HookBeforeRemoveOp runs before an operation directory is drained.
	HookBeforeRemoveOp
	// HookAfterRemoveOp runs after an operation directory was drained,
	// before its absence is re-checked.
	HookAfterRemoveOp
	// HookBeforeMoveRollback runs after a move's destination proof failed,
	// before the rollback authority is re-proven from a fresh open of the
	// destination.
	HookBeforeMoveRollback
	// HookAfterMoveRename runs after the forward rename completed and both
	// parents were synced, before the destination is opened for its proof.
	HookAfterMoveRename
	// HookAfterRemoveSkillLiveMoved runs after IsolateRemove parked the
	// live tree into the operation container, before Baseline/previous
	// isolation. Tests fail a later step and assert the live tree is restored.
	HookAfterRemoveSkillLiveMoved
	// HookBeforeTransientDrain runs before a transient slot (a moved-aside
	// Baseline or stale previous snapshot) is drained with its retained
	// identity.
	HookBeforeTransientDrain
	// HookAfterTransientDrain runs after a transient slot was drained,
	// before its final absence is re-checked.
	HookAfterTransientDrain
	// HookAfterOldLiveMoved runs after a replace parked the old live tree
	// in the recovery slot, before the staged tree is renamed into the
	// live name.
	HookAfterOldLiveMoved
)

// Operation is one durable Store mutation intent: the journal record that
// makes live-tree replacement crash-recoverable. It is persisted in SQLite
// (state) before any filesystem mutation, and every transition references
// it by ID. Receipt carries the opaque terminal receipt bytes of a
// finalized or restored row; the application persists and passes them
// through without interpreting them.
type Operation struct {
	ID        int64  // the journal id; names the operation's staging and recovery paths
	SkillID   int64  // the committed Skill id (0 while a fresh import is pending)
	Slug      string // the live Skill directory name
	Kind      string // KindImport or KindReplace
	OldDigest string // the live digest before a replace ("" for imports, and "" for a replace that displaces non-tree content that has no digest)
	NewDigest string // the digest of the content being installed
	Phase     string // PhasePending, PhaseCommitted, PhaseAborted, PhaseFinalized, or PhaseRestored
	Receipt   []byte // the opaque terminal receipt of a finalized/restored row
	// BaselineMode is BaselineAdvance or BaselineKeep; Install skips the
	// Baseline candidate and Finalize retains the existing Baseline for
	// BaselineKeep operations (rollback).
	BaselineMode string
	// BaselineDigest is the expected pre-operation Baseline digest when it
	// differs from the displaced live digest; empty means the
	// import-replace invariant. Accept Source records it here instead of
	// mutating the Baseline up front: only the committed replacement
	// advances it.
	BaselineDigest string
}

// displacedBaselineDigest is the expected pre-operation Baseline digest:
// the intent-recorded BaselineDigest (an Accept Source repair, or a replace
// over a locally diverged Baseline), or the displaced live digest itself.
func (op Operation) displacedBaselineDigest() string {
	if op.BaselineDigest != "" {
		return op.BaselineDigest
	}
	return op.OldDigest
}

// oldUnreadable reports whether a replace displaced live content that is
// not a digestable Skill tree: the journal moves it by identity only, the
// finalize removes it instead of rotating a snapshot, and a pending restore
// returns it by identity.
func (op Operation) oldUnreadable() bool {
	return op.Kind == KindReplace && op.OldDigest == ""
}

// Store is the Skill Store tree root. All methods assume the caller holds
// the Store exclusive lock for the whole operation.
type Store struct {
	Root string

	// hook is the deterministic test seam for the Stage-A invariants; nil
	// in production.
	hook func(HookPoint)
}

// runHook invokes the test seam at the given point, if set.
func (s Store) runHook(p HookPoint) {
	if s.hook != nil {
		s.hook(p)
	}
}

// SetHook installs the deterministic test seam (nil in production). It is
// exported so the application-boundary tests can exercise the same narrow
// race windows as the Store's own tests.
func (s *Store) SetHook(h func(HookPoint)) {
	s.hook = h
}

// New returns the Store rooted at root.
func New(root string) Store {
	return Store{Root: root}
}

// SkillDir is the live directory of one Skill. The slug is validated before
// it is joined into a path, so a slug can never escape the Store root or
// target the internal .skillctl layout.
func (s Store) SkillDir(slug string) (string, error) {
	if err := domain.ValidateSlug(slug); err != nil {
		return "", err
	}
	return filepath.Join(s.Root, slug), nil
}

// InternalDir is the internal .skillctl layout, excluded from Skill
// enumeration, import, and Distribution.
func (s Store) InternalDir() string {
	return filepath.Join(s.Root, ".skillctl")
}

// BaselineDir is the immutable Synchronization Baseline tree of one Skill.
func (s Store) BaselineDir(skillID int64) string {
	return filepath.Join(s.InternalDir(), "baselines", strconv.FormatInt(skillID, 10))
}

// PreviousDir is the single previous snapshot of one Skill.
func (s Store) PreviousDir(skillID int64) string {
	return filepath.Join(s.InternalDir(), "previous", strconv.FormatInt(skillID, 10))
}

func (s Store) opRel(opID int64) string {
	return path.Join(".skillctl", "staging", strconv.FormatInt(opID, 10))
}

func (s Store) opDir(opID int64) string {
	return filepath.Join(s.Root, filepath.FromSlash(s.opRel(opID)))
}

func (s Store) stagedTreeDir(opID int64) string {
	return filepath.Join(s.opDir(opID), "tree")
}

func (s Store) baselineCandidateDir(opID int64) string {
	return filepath.Join(s.opDir(opID), "baseline")
}

func (s Store) baselineBackupDir(opID int64) string {
	return filepath.Join(s.opDir(opID), "baseline-old")
}

func (s Store) recoveryDir(opID int64) string {
	return filepath.Join(s.InternalDir(), "recovery", strconv.FormatInt(opID, 10))
}

// EnsureLayout lazily creates the Store root and the internal .skillctl
// layout before the first Store write. Every component is created and
// pinned one at a time through an already-open parent handle, so a relative
// or absolute .skillctl symlink raced into place between components can
// never redirect the creation of staging/baselines/previous/recovery into
// a live Skill subtree. Every logical name is re-proven before success.
func (s Store) EnsureLayout() error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	info, err := os.Lstat(s.Root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("Skill Store root is not a directory")
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	rid, rerr := rootID(root)
	if rerr != nil {
		return rerr
	}
	if fid, ferr := fileIDOf(info); ferr != nil || fid != rid {
		return fmt.Errorf("Skill Store root changed while it was being opened")
	}
	internal, internalID, err := ensurePinnedDir(root, ".skillctl", 0o755)
	if err != nil {
		return err
	}
	defer internal.Close()
	childIDs := map[string]fileID{}
	for _, name := range []string{"staging", "baselines", "previous", "recovery"} {
		child, childID, err := ensurePinnedDir(internal, name, 0o755)
		if err != nil {
			return err
		}
		childID, err = rootID(child)
		child.Close()
		if err != nil {
			return err
		}
		childIDs[name] = childID
	}
	// Revalidate every logical name before returning success: the Store
	// root must still identify the opened root, .skillctl must still
	// identify the pinned internal handle, and every internal child name
	// must still identify its pinned directory, so a swap at any level
	// during layout creation is never blessed as the layout.
	s.runHook(HookBeforeEnsureLayoutVerify)
	if info, err := os.Lstat(s.Root); err != nil {
		return err
	} else if fid, ferr := fileIDOf(info); ferr != nil || fid != rid {
		return fmt.Errorf("Skill Store root changed while the layout was being created")
	}
	if err := nameRefersTo(root, ".skillctl", internalID); err != nil {
		return err
	}
	for _, name := range []string{"staging", "baselines", "previous", "recovery"} {
		if err := nameRefersTo(internal, name, childIDs[name]); err != nil {
			return err
		}
	}
	return nil
}

// DiscardStaging removes one operation's staging through the opaque proof
// Stage handed back: the exact creation manifest of that Stage invocation
// authorizes every removal leaf→root, and the operation directory must
// still carry the identity Stage proved. An expected operation directory
// that is missing, an identity or name mismatch, an unknown child, or a
// reappeared operation directory is ErrAmbiguous with everything preserved:
// a fresh sample of the operation name never authorizes deletion. Only
// after every recorded object is provably gone and the final absence is
// re-checked does DiscardStaging succeed. It is safe only when the caller
// can prove Install never mutated the live path; any pre-commit failure
// after a live mutation must go through Restore instead, so unprovable
// state stays preserved for recovery.
func (s Store) DiscardStaging(opID int64, staged StagedProof) error {
	if staged.opID != opID || staged.ownership == nil {
		return errWrap(ErrAmbiguous, "staged proof does not bind operation %d", opID)
	}
	if err := s.EnsureLayout(); err != nil {
		return err
	}
	layout, err := s.openLayout()
	if err != nil {
		return errWrap(ErrAmbiguous, "opening the Store layout for the discard: %v", err)
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return err
	}
	if err := cleanupOwnership(layout.staging, opID, staged.ownership, s.hook); err != nil {
		return err
	}
	return layout.verify()
}

// VerifyBaselineDigest requires the Baseline tree of one Skill to carry
// exactly the recorded digest (or to be absent when the recorded digest is
// empty): it is the pre-commit authority check of a Baseline-only refresh,
// so a foreign or tampered Baseline fails the refresh cleanly before the
// journal commits. Anything else is ErrAmbiguous with the tree preserved.
func (s Store) VerifyBaselineDigest(ctx context.Context, skillID int64, digest string) error {
	layout, err := s.openLayout()
	if err != nil {
		return err
	}
	defer layout.close()
	if err := layout.verify(); err != nil {
		return err
	}
	base, _, baseDigest, err := openTreeDigest(ctx, layout.baselines, strconv.FormatInt(skillID, 10))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if digest == "" {
				return nil
			}
			return errWrap(ErrAmbiguous, "the Baseline of Skill %d is missing; the refresh is refused", skillID)
		}
		return errWrap(ErrAmbiguous, "reading the Baseline of Skill %d: %v", skillID, err)
	}
	base.Close()
	if baseDigest != digest {
		return errWrap(ErrAmbiguous, "the Baseline of Skill %d carries digest %s, not the recorded %s; it is preserved", skillID, baseDigest, digest)
	}
	return nil
}

// errWrap attaches context to a sentinel error while preserving it for
// errors.Is.
func errWrap(sentinel error, format string, args ...any) error {
	return fmt.Errorf("%w: %s", sentinel, fmt.Sprintf(format, args...))
}
