package app

import (
	"fmt"
	"sort"

	"skillctl/internal/domain"
	"skillctl/internal/source"
)

// ImportSkillsInput is one batch import request: exactly one Source, either
// every Inventory entry (All) or an explicit selector list. AllowLarge is
// the batch-level override for the per-Skill size and file-count guards.
type ImportSkillsInput struct {
	SourceID   int64
	Selectors  []ImportSelector
	All        bool
	AllowLarge bool
}

// ImportSelector selects one Source Inventory entry either by its exact
// relative directory or by its Inventory name (accepted only when unique
// within the Source). Slug is an explicit slug override: nil means "derive
// from the frontmatter name", a non-nil value is validated unchanged.
// Replace is the explicit per-entry permission to replace the existing
// managed Skill claiming the final slug.
type ImportSelector struct {
	RelativeDir string
	Name        string
	Slug        *string
	Replace     bool
}

// validateImportInput rejects invalid batch combinations before any
// observation or Store work: the Source id must be present, exactly one of
// All or a non-empty selector list must be given (so Replace and slug
// overrides are impossible with All), every selector must choose exactly
// one of path or name, exact duplicate selectors are refused, and every
// explicit slug override must pass the strict slug grammar unchanged.
func validateImportInput(in ImportSkillsInput) error {
	if in.SourceID <= 0 {
		return fmt.Errorf("a Source id is required")
	}
	if in.All && len(in.Selectors) > 0 {
		return fmt.Errorf("selectors cannot be combined with --all")
	}
	if !in.All && len(in.Selectors) == 0 {
		return fmt.Errorf("select at least one Skill entry or --all")
	}
	for i := range in.Selectors {
		sel := &in.Selectors[i]
		switch {
		case sel.RelativeDir != "" && sel.Name != "":
			return fmt.Errorf("selector %d: choose either a relative path or a name, not both", i+1)
		case sel.RelativeDir == "" && sel.Name == "":
			return fmt.Errorf("selector %d: a relative path or a name is required", i+1)
		}
		if sel.Slug != nil {
			if err := domain.ValidateSlug(*sel.Slug); err != nil {
				return fmt.Errorf("selector %d: %v", i+1, err)
			}
		}
	}
	seen := map[string]bool{}
	for i := range in.Selectors {
		key := "path:" + in.Selectors[i].RelativeDir
		if in.Selectors[i].RelativeDir == "" {
			key = "name:" + in.Selectors[i].Name
		}
		if seen[key] {
			return fmt.Errorf("duplicate selection of %q", key)
		}
		seen[key] = true
	}
	return nil
}

// resolveImportSelectors maps a validated request onto Source Inventory
// entries. All expands to every entry sorted by relative directory;
// explicit selectors keep their request order. A path matches the exact
// relative directory; a name must be unique within the Inventory, and a
// path and a name resolving to the same entry are rejected as a duplicate
// selection. The input must already have passed validateImportInput.
func resolveImportSelectors(in ImportSkillsInput, entries []source.Entry) ([]source.Entry, error) {
	if in.All {
		out := append([]source.Entry(nil), entries...)
		sort.Slice(out, func(i, j int) bool { return out[i].RelativeDir < out[j].RelativeDir })
		return out, nil
	}
	var out []source.Entry
	claimed := map[string]bool{}
	for i, sel := range in.Selectors {
		var entry *source.Entry
		if sel.RelativeDir != "" {
			for j := range entries {
				if entries[j].RelativeDir == sel.RelativeDir {
					entry = &entries[j]
					break
				}
			}
			if entry == nil {
				return nil, fmt.Errorf("selector %d: no Inventory entry at %q", i+1, sel.RelativeDir)
			}
		} else {
			var matches []*source.Entry
			for j := range entries {
				if entries[j].Name == sel.Name {
					matches = append(matches, &entries[j])
				}
			}
			switch len(matches) {
			case 0:
				return nil, fmt.Errorf("selector %d: no Inventory entry named %q", i+1, sel.Name)
			case 1:
				entry = matches[0]
			default:
				return nil, fmt.Errorf("selector %d: name %q is ambiguous; use its relative path", i+1, sel.Name)
			}
		}
		if claimed[entry.RelativeDir] {
			return nil, fmt.Errorf("duplicate selection of %q", entry.RelativeDir)
		}
		claimed[entry.RelativeDir] = true
		out = append(out, *entry)
	}
	return out, nil
}
