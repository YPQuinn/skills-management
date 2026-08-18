package distribution

import (
	"fmt"
	"os"
	"path/filepath"

	"skillctl/internal/target"
)

// Gate states: the re-verified physical Target identity.
const (
	StateOK         = "ok"
	StateMissing    = "missing"
	StateRedirected = "redirected"
	StateInvalid    = "invalid"
)

// Gate is the result of re-verifying one registered Target before
// inspection becomes mutation (decision 06).
type Gate struct {
	State string
	Path  string // the current physical container path (ok/missing)
	Error string
}

// GateTarget re-resolves every existing path prefix of the registered
// Target and re-verifies the decision-04/06 safety boundaries: the
// physical identity must still match registration, the filesystem root
// cannot be a Target, the Target and Skill Store must remain disjoint,
// and an existing container must be a directory. A missing container is
// the normal missing state; Distribution may create it. The Store path is
// resolved the same way so both sides are compared at their physical
// locations.
func GateTarget(targetPath, storePath string, o target.ResolveOptions) Gate {
	if o.Stat == nil {
		o.Stat = os.Stat
	}
	abs, err := filepath.Abs(targetPath)
	if err != nil {
		return Gate{State: StateInvalid, Error: "resolving the Target path: " + err.Error()}
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return Gate{State: StateInvalid, Error: "the filesystem root cannot be a Target"}
	}
	physical, err := target.ResolvePhysicalPath(abs, o)
	if err != nil {
		return Gate{State: StateInvalid, Error: "resolving the Target path: " + err.Error()}
	}
	if physical != abs {
		return Gate{State: StateRedirected, Path: physical,
			Error: fmt.Sprintf("the Target path now resolves to %s; re-register it to continue", physical)}
	}
	if err := target.CheckStoreDisjoint(physical, storePath, o); err != nil {
		return Gate{State: StateInvalid, Path: physical, Error: err.Error()}
	}
	info, err := o.Stat(physical)
	if err == nil {
		if !info.IsDir() {
			return Gate{State: StateInvalid, Path: physical, Error: "the Target container is not a directory"}
		}
		return Gate{State: StateOK, Path: physical}
	}
	if os.IsNotExist(err) {
		return Gate{State: StateMissing, Path: physical}
	}
	return Gate{State: StateInvalid, Path: physical, Error: "inspecting the Target container: " + err.Error()}
}
