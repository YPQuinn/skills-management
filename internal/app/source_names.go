package app

import (
	"fmt"
	"strings"
	"time"

	"skillctl/internal/source"
	"skillctl/internal/state"
)

// validateSourceName checks an operator-facing Source name: it must survive
// trimming, must be safe as one URL path segment, and must not be the
// ambiguous "." or ".." segments. Uniqueness is enforced by the state
// schema, not here.
func validateSourceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("Source name must not be empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("Source name must not contain %q", "/")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("Source name must not be %q", name)
	}
	return nil
}

// defaultSourceName derives a display name from a normalized location: the
// last path or scp-style segment without a trailing .git.
func defaultSourceName(loc source.Locator) string {
	base := strings.TrimSuffix(loc.Location, "/")
	base = strings.TrimSuffix(base, ".git")
	if i := strings.LastIndexAny(base, "/:"); i >= 0 {
		base = base[i+1:]
	}
	if base == "" {
		return "source"
	}
	return base
}

// RenameSource changes the operator-facing name of one Source. An empty
// name is invalid (it does not fall back to the location default). The
// same name after trimming is a no-op and does not write. Inventory,
// location, and bindings are unchanged.
func (a *App) RenameSource(id int64, name string) (*source.Source, error) {
	current, err := a.ShowSource(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if err := validateSourceName(name); err != nil {
		return nil, Errorf(CodeInvalidArgument, "%v", err)
	}
	if name == current.Name {
		return current, nil
	}
	now := time.Now().UTC()
	if err := state.UpdateSourceName(a.db, id, name, now); err != nil {
		if state.IsUniqueViolation(err) {
			return nil, Errorf(CodeConflict, "a Source named %q is already registered", name)
		}
		return nil, Errorf(CodeInternal, "renaming Source %d: %v", id, err)
	}
	return a.ShowSource(id)
}
