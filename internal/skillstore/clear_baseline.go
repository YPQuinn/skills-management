package skillstore

import (
	"errors"
	"os"
	"strconv"

	"skillctl/internal/domain"
)

// IsolateBaseline parks one Skill's Baseline tree into recovery/<opID>.
// Absence is a no-op. Used by a pending BaselineClear journal.
func (s Store) IsolateBaseline(op Operation) error {
	if err := checkBaselineClear(op); err != nil {
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
	slot := strconv.FormatInt(op.ID, 10)
	_, err = parkNamedDir(layout, layout.baselines, strconv.FormatInt(op.SkillID, 10), layout.recovery, slot)
	return err
}

// RestoreBaseline returns a parked BaselineClear tree to baselines/<id>.
func (s Store) RestoreBaseline(op Operation) error {
	if err := checkBaselineClear(op); err != nil {
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
	return restoreSlot(layout, layout.recovery, strconv.FormatInt(op.ID, 10), layout.baselines, strconv.FormatInt(op.SkillID, 10))
}

// FinalizeBaselineClear drains the parked Baseline of a committed
// BaselineClear journal. Missing is success.
func (s Store) FinalizeBaselineClear(op Operation) error {
	if err := checkBaselineClear(op); err != nil {
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
	return drainNamedDir(layout.recovery, strconv.FormatInt(op.ID, 10))
}

func checkBaselineClear(op Operation) error {
	if err := domain.ValidateSlug(op.Slug); err != nil {
		return err
	}
	if op.Kind != KindBaseline || op.BaselineMode != BaselineClear {
		return errWrap(ErrAmbiguous, "operation %d is not a Baseline clear", op.ID)
	}
	if op.SkillID <= 0 {
		return errWrap(ErrAmbiguous, "operation %d has no Skill id", op.ID)
	}
	return nil
}
