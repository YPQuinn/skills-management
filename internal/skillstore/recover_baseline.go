package skillstore

import (
	"context"
	"errors"
	"os"

	"skillctl/internal/domain"
)

// prepareRestoreBaseline unwinds a pending Baseline-only refresh. The swap
// only ever runs during the finalization of a committed refresh, so a
// pending refresh never mutated anything outside its staging: the
// never-started state (no operation evidence at all) is the clearable
// intent the caller certifies through the abort protocol, and surviving
// staging is preserved exactly like the import contract — without a
// persisted physical proof the current sample is never attributed to the
// operation and never removed.
func (s Store) prepareRestoreBaseline(ctx context.Context, op Operation) (CleanupReceipt, error) {
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return CleanupReceipt{}, err
	}
	layout, err := s.openLayout()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return CleanupReceipt{}, errWrap(ErrAmbiguous, "opening the Store layout: %v", err)
		}
		return CleanupReceipt{}, nil
	}
	defer layout.close()
	s.runHook(HookAfterLayoutOpen)
	if err := layout.verify(); err != nil {
		return CleanupReceipt{}, err
	}
	opDir, err := layout.opDir(op.ID)
	switch {
	case err == nil:
		opDir.Close()
		// A pending refresh whose staging survives cannot be attributed
		// across a restart; every candidate is preserved for the operator,
		// and no Store write proceeds until it resolves.
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "pending operation %d has surviving staging with no persisted physical proof; it is preserved", op.ID)
	case errors.Is(err, os.ErrNotExist):
		return CleanupReceipt{}, nil
	default:
		return CleanupReceipt{}, errWrap(ErrAmbiguous, "opening operation %d staging: %v", op.ID, err)
	}
}
