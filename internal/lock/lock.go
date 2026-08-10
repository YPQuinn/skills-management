// Package lock provides minimal non-blocking cross-process file locks that
// coordinate writers across simultaneous skillctl processes. Lock failures
// surface immediately as ErrLocked; callers retry explicitly, there are no
// hidden retries.
package lock

import (
	"errors"

	"github.com/gofrs/flock"
)

// ErrLocked reports that the lock is held by another process.
var ErrLocked = errors.New("lock is held by another process")

// Lock is a held cross-process file lock. Release it with Unlock.
type Lock struct {
	f *flock.Flock
}

// TryExclusive acquires an exclusive (writer) lock without blocking.
func TryExclusive(path string) (*Lock, error) {
	return try(path, false)
}

// TryShared acquires a shared (reader) lock without blocking.
func TryShared(path string) (*Lock, error) {
	return try(path, true)
}

func try(path string, shared bool) (*Lock, error) {
	f := flock.New(path)
	var ok bool
	var err error
	if shared {
		ok, err = f.TryRLock()
	} else {
		ok, err = f.TryLock()
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLocked
	}
	return &Lock{f: f}, nil
}

// Unlock releases the lock.
func (l *Lock) Unlock() error {
	return l.f.Unlock()
}
