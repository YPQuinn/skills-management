package app

import (
	"fmt"
	"strings"
)

// defaultSlug derives a decision-03-compatible slug from a Skill's
// frontmatter name: ASCII uppercase is lowercased, ASCII letters and digits
// are preserved, and every run of any other character collapses into a
// single hyphen, with leading and trailing separators trimmed. Non-ASCII
// characters are not transliterated. When the result is empty or exceeds
// the 64-character limit, the error tells the operator to provide an
// explicit slug instead of guessing.
func defaultSlug(name string) (string, error) {
	var b strings.Builder
	pendingHyphen := false
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteByte(byte(r) + ('a' - 'A'))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
		default:
			pendingHyphen = true
		}
	}
	slug := b.String()
	if slug == "" {
		return "", fmt.Errorf("name %q produces an empty slug; provide an explicit slug", name)
	}
	if len(slug) > 64 {
		return "", fmt.Errorf("name %q produces a %d-character slug, exceeding the 64-character limit; provide an explicit slug", name, len(slug))
	}
	return slug, nil
}
