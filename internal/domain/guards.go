package domain

import "fmt"

// Per-Skill import guards (decision 03): at most 10,000 files, 100 MiB
// total, and 50 MiB per file. allow-large bypasses these limits; unsafe
// paths and special nodes can never be overridden.
const (
	MaxFilesPerSkill = 10_000
	MaxBytesPerSkill = 100 << 20
	MaxBytesPerFile  = 50 << 20
)

// Guard enforces the per-Skill limits while content is materialized from a
// Source and again while it is staged into the Store. Materialization
// checks each file before its bytes are read or fetched; staging re-checks
// the limits as defense at the Store boundary.
type Guard struct {
	AllowLarge bool
	files      int
	bytes      int64
}

// AddFile checks and records one regular file of the given size, returning
// a stable, actionable error when a limit is exceeded. With AllowLarge the
// limits are bypassed entirely.
func (g *Guard) AddFile(size int64) error {
	if g.AllowLarge {
		return nil
	}
	g.files++
	if g.files > MaxFilesPerSkill {
		return fmt.Errorf("Skill exceeds the %d-file limit; re-import with allow-large", MaxFilesPerSkill)
	}
	if size > MaxBytesPerFile {
		return fmt.Errorf("a file exceeds the %d-byte per-file limit; re-import with allow-large", MaxBytesPerFile)
	}
	g.bytes += size
	if g.bytes > MaxBytesPerSkill {
		return fmt.Errorf("Skill exceeds the %d-byte total limit; re-import with allow-large", MaxBytesPerSkill)
	}
	return nil
}
