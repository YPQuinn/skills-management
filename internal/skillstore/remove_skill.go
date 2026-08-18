package skillstore

import (
	"errors"
	"os"
	"strconv"

	"skillctl/internal/domain"
)

const (
	removeSlotLive = "live"
	removeSlotBase = "base"
	removeSlotPrev = "prev"
)

// IsolateRemove parks the live tree, Baseline, and previous snapshot under
// recovery/<opID>/. Isolation is a rename, so a later pending restore can
// put every still-parked tree back. A missing tree is a no-op; a
// non-directory at the live slug is unmanaged and is left untouched.
func (s Store) IsolateRemove(op Operation) error {
	if op.Kind != KindRemove {
		return errWrap(ErrAmbiguous, "operation %d is not a remove", op.ID)
	}
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return err
	}
	if err := s.EnsureLayout(); err != nil {
		return err
	}
	layout, err := s.openLayout()
	if err != nil {
		return err
	}
	defer layout.close()
	box, err := openOrCreateRemoveBox(layout, op.ID)
	if err != nil {
		return err
	}
	defer box.Close()
	idName := strconv.FormatInt(op.SkillID, 10)
	live, err := parkNamedDir(layout, layout.store, op.Slug, box, removeSlotLive)
	if err != nil {
		return restoreParked(layout, err, live)
	}
	s.runHook(HookAfterRemoveSkillLiveMoved)
	base, err := parkNamedDir(layout, layout.baselines, idName, box, removeSlotBase)
	if err != nil {
		return restoreParked(layout, err, live)
	}
	prev, err := parkNamedDir(layout, layout.previous, idName, box, removeSlotPrev)
	if err != nil {
		return restoreParked(layout, err, live, base)
	}
	_ = prev
	return nil
}

// RestoreRemove returns every parked remove tree to its original path.
// Absence of the operation container or of a slot is success (never started
// or already restored). A leftover unexpected child is preserved.
func (s Store) RestoreRemove(op Operation) error {
	if op.Kind != KindRemove {
		return errWrap(ErrAmbiguous, "operation %d is not a remove", op.ID)
	}
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return err
	}
	layout, err := s.openLayout()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer layout.close()
	box, err := openLayoutDir(layout.recovery, strconv.FormatInt(op.ID, 10))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer box.Close()
	idName := strconv.FormatInt(op.SkillID, 10)
	if err := restoreSlot(layout, box, removeSlotLive, layout.store, op.Slug); err != nil {
		return err
	}
	if err := restoreSlot(layout, box, removeSlotBase, layout.baselines, idName); err != nil {
		return err
	}
	if err := restoreSlot(layout, box, removeSlotPrev, layout.previous, idName); err != nil {
		return err
	}
	return removeEmptyBox(layout, box, op.ID)
}

// FinalizeRemove drains parked remove trees. It runs only after the
// journal committed and the Skill row is gone. Missing slots are success.
func (s Store) FinalizeRemove(op Operation) error {
	if op.Kind != KindRemove {
		return errWrap(ErrAmbiguous, "operation %d is not a remove", op.ID)
	}
	if err := s.EnsureLayout(); err != nil {
		return err
	}
	layout, err := s.openLayout()
	if err != nil {
		return err
	}
	defer layout.close()
	box, err := openLayoutDir(layout.recovery, strconv.FormatInt(op.ID, 10))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer box.Close()
	for _, slot := range []string{removeSlotPrev, removeSlotBase, removeSlotLive} {
		if err := drainNamedDir(box, slot); err != nil {
			return err
		}
	}
	return removeEmptyBox(layout, box, op.ID)
}

// RemoveBaseline drains one Skill's Baseline tree. Missing is a no-op.
func (s Store) RemoveBaseline(skillID int64) error {
	if err := s.EnsureLayout(); err != nil {
		return err
	}
	layout, err := s.openLayout()
	if err != nil {
		return err
	}
	defer layout.close()
	return drainNamedDir(layout.baselines, strconv.FormatInt(skillID, 10))
}

func openOrCreateRemoveBox(layout *storeLayout, opID int64) (*os.Root, error) {
	name := strconv.FormatInt(opID, 10)
	if exists, err := childExists(layout.recovery, name); err != nil {
		return nil, err
	} else if exists {
		return openLayoutDir(layout.recovery, name)
	}
	box, _, err := createPinnedDirExclusive(layout.recovery, name, 0o755)
	if err != nil {
		return nil, err
	}
	if err := syncRoot(layout.recovery); err != nil {
		box.Close()
		return nil, errWrap(ErrAmbiguous, "recovery parent could not be synced after creating operation %d: %v", opID, err)
	}
	return box, nil
}

func removeEmptyBox(layout *storeLayout, box *os.Root, opID int64) error {
	dir, err := box.Open(".")
	if err != nil {
		return err
	}
	ents, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		return err
	}
	if len(ents) > 0 {
		return errWrap(ErrAmbiguous, "remove operation %d still has unexpected recovery children", opID)
	}
	info, err := layout.recovery.Lstat(strconv.FormatInt(opID, 10))
	if err != nil {
		return err
	}
	id, err := fileIDOf(info)
	if err != nil {
		return err
	}
	var seq uint64
	return deleteVerified(layout.recovery, strconv.FormatInt(opID, 10), id, &seq)
}

func restoreSlot(layout *storeLayout, box *os.Root, slot string, dest *os.Root, name string) error {
	info, err := box.Lstat(slot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errWrap(ErrAmbiguous, "recovery slot %s is not a directory", slot)
	}
	id, err := fileIDOf(info)
	if err != nil {
		return err
	}
	return parkedDir{parent: dest, dest: box, name: name, slot: slot, id: id, moved: true}.restore(layout)
}
