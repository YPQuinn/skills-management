package distribution

import (
	"errors"
)

// ErrEntryChanged is returned when a symlink's raw target or physical
// identity changed between the paired reads used to sample it.
var ErrEntryChanged = errors.New("the Target entry changed during the read")

// ErrLinkMismatch is returned when a Target entry is not the recorded
// Managed Link (wrong kind, raw target, or physical identity).
var ErrLinkMismatch = errors.New("the Target entry is not the recorded Managed Link")

// ErrNotSymlink is returned when a sampled Target entry is not a symlink.
var ErrNotSymlink = errors.New("the entry is not a symlink")

// LinkProof is the raw target and physical identity of one symlink.
type LinkProof struct {
	Raw   string
	Dev   uint64
	Ino   uint64
	Mtime int64
}

func (p LinkProof) Proven() bool {
	return p.Raw != "" && p.Dev != 0 && p.Ino != 0
}

func (p LinkProof) Matches(got LinkProof) bool {
	return p.Proven() && p.Raw == got.Raw && p.Dev == got.Dev && p.Ino == got.Ino && p.Mtime == got.Mtime
}

// VerifyLink reports whether slug is still the recorded Managed Link.
func VerifyLink(containerPath, slug string, proof LinkProof) error {
	got, err := ProbeSymlink(containerPath, slug)
	if err != nil {
		if errors.Is(err, ErrNotSymlink) || errors.Is(err, ErrEntryChanged) {
			return ErrLinkMismatch
		}
		return err
	}
	if !proof.Matches(got) {
		return ErrLinkMismatch
	}
	return nil
}

// testAfterSymlinkStat is a test-only seam between the first identity
// sample and the confirming re-read.
var testAfterSymlinkStat func()

func (c *containerHandle) probeStableSymlink(name string) (LinkProof, error) {
	st, err := c.stat(name)
	if err != nil {
		return LinkProof{}, err
	}
	if statKind(st) != KindSymlink {
		return LinkProof{}, ErrNotSymlink
	}
	if testAfterSymlinkStat != nil {
		testAfterSymlinkStat()
	}
	raw, err := c.readlink(name)
	if err != nil {
		return LinkProof{}, err
	}
	st2, err := c.stat(name)
	if err != nil {
		return LinkProof{}, err
	}
	raw2, err := c.readlink(name)
	if err != nil {
		return LinkProof{}, err
	}
	if statKind(st2) != KindSymlink {
		return LinkProof{}, ErrEntryChanged
	}
	dev, ino, mtime := symlinkIdentity(st)
	dev2, ino2, mtime2 := symlinkIdentity(st2)
	if dev != dev2 || ino != ino2 || mtime != mtime2 || raw != raw2 {
		return LinkProof{}, ErrEntryChanged
	}
	return LinkProof{Raw: raw2, Dev: dev2, Ino: ino2, Mtime: mtime2}, nil
}
