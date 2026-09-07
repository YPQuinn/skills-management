package distribution

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	isolationPrefix = ".skillctl-r-"
	// IsolatedEntry is the fixed name of the isolated Managed Link inside
	// the private isolation directory. Operations after isolation use this
	// name relative to the pinned isolation-directory fd.
	IsolatedEntry = "link"
)

// NewIsolationName returns one unguessable single-component directory
// name for create staging or remove isolation. It is persisted before
// mkdirat; it is never a visible symlink at the Target root.
func NewIsolationName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%x", isolationPrefix, b), nil
}

func validIsolationName(name string) bool {
	return validEntryName(name) && strings.HasPrefix(name, isolationPrefix)
}
