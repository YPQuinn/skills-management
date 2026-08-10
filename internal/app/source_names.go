package app

import (
	"fmt"
	"strings"

	"skillctl/internal/source"
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
