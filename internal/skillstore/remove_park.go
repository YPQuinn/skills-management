package skillstore

import (
	"errors"
	"os"
)

// parkedDir is one tree renamed into an operation-owned recovery slot.
type parkedDir struct {
	parent *os.Root
	dest   *os.Root
	name   string
	slot   string
	id     fileID
	moved  bool
}

// parkNamedDir renames one directory into dest/slot. Absence is a no-op;
// a non-directory is unmanaged; a slot that already exists is refused so a
// leftover isolation is never overwritten.
func parkNamedDir(layout *storeLayout, parent *os.Root, name string, dest *os.Root, slot string) (parkedDir, error) {
	p := parkedDir{parent: parent, dest: dest, name: name, slot: slot}
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if !info.IsDir() {
		return p, errWrap(ErrUnmanaged, "%s is not a Skill directory", name)
	}
	id, err := fileIDOf(info)
	if err != nil {
		return p, err
	}
	if exists, err := childExists(dest, slot); err != nil {
		return p, err
	} else if exists {
		return p, errWrap(ErrAmbiguous, "recovery slot %s already exists", slot)
	}
	if err := nameRefersTo(parent, name, id); err != nil {
		return p, errWrap(ErrAmbiguous, "%s changed before it was isolated: %v", name, err)
	}
	if err := renameNoReplaceAt(parent, name, dest, slot); err != nil {
		return p, err
	}
	if err := syncRoot(parent); err != nil {
		return p, errWrap(ErrAmbiguous, "%s was isolated but its parent could not be synced: %v", name, err)
	}
	if err := syncRoot(dest); err != nil {
		return p, errWrap(ErrAmbiguous, "%s was isolated but recovery could not be synced: %v", name, err)
	}
	if err := nameRefersTo(dest, slot, id); err != nil {
		return p, errWrap(ErrAmbiguous, "isolated %s cannot be proven: %v", slot, err)
	}
	p.id = id
	p.moved = true
	return p, nil
}

func restoreParked(layout *storeLayout, cause error, parks ...parkedDir) error {
	for i := len(parks) - 1; i >= 0; i-- {
		if err := parks[i].restore(layout); err != nil {
			return errWrap(ErrAmbiguous, "remove failed (%v) and the Store Skill could not be restored: %v", cause, err)
		}
	}
	return cause
}

func (p parkedDir) restore(layout *storeLayout) error {
	if !p.moved {
		return nil
	}
	occupied, err := childExists(p.parent, p.name)
	if err != nil {
		return err
	}
	if occupied {
		return errWrap(ErrAmbiguous, "%s is occupied; the isolated tree remains in %s", p.name, p.slot)
	}
	if err := nameRefersTo(p.dest, p.slot, p.id); err != nil {
		return errWrap(ErrAmbiguous, "isolated %s cannot be proven: %v", p.slot, err)
	}
	if err := renameNoReplaceAt(p.dest, p.slot, p.parent, p.name); err != nil {
		return errWrap(ErrAmbiguous, "isolated %s could not be restored to %s: %v", p.slot, p.name, err)
	}
	if err := syncRoot(p.dest); err != nil {
		return errWrap(ErrAmbiguous, "%s was restored but recovery could not be synced: %v", p.name, err)
	}
	if err := syncRoot(p.parent); err != nil {
		return errWrap(ErrAmbiguous, "%s was restored but its parent could not be synced: %v", p.name, err)
	}
	if err := nameRefersTo(p.parent, p.name, p.id); err != nil {
		return errWrap(ErrAmbiguous, "restored %s cannot be proven: %v", p.name, err)
	}
	return nil
}

// drainNamedDir removes one single-component directory below a pinned
// parent. Absence is success; a non-directory is unmanaged.
func drainNamedDir(parent *os.Root, name string) error {
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errWrap(ErrUnmanaged, "%s is not a Skill directory", name)
	}
	id, err := fileIDOf(info)
	if err != nil {
		return err
	}
	return drainTree(parent, name, id)
}
