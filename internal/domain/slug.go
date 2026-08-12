package domain

import (
	"fmt"
	"regexp"
)

// slugRe is the decision-03 slug grammar: lowercase ASCII letters and
// digits joined by single hyphens. The grammar inherently rejects empty
// slugs, dot names (including "." and ".."), separators, the ".skillctl"
// internal directory, and any other character outside [a-z0-9-].
var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateSlug enforces the decision-03 slug grammar at a filesystem
// boundary: a slug must be 1-64 characters of lowercase ASCII letters and
// digits joined by single hyphens, so it is always a safe single directory
// name and URL segment. Every Store path built from a slug validates it
// first; a persisted operation with an invalid slug is rejected rather than
// joined into a path.
func ValidateSlug(slug string) error {
	if len(slug) == 0 {
		return fmt.Errorf("slug is required")
	}
	if len(slug) > 64 {
		return fmt.Errorf("slug %q exceeds the 64-character limit", slug)
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase ASCII letters and digits joined by single hyphens", slug)
	}
	return nil
}
