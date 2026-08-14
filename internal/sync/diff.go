package sync

import (
	"sort"
	"strings"
)

// Change kinds of one path entry (decision 05): additions, deletions,
// content changes, executable-bit changes, and node-type changes.
const (
	ChangeAdd      = "add"
	ChangeDelete   = "delete"
	ChangeContent  = "content"
	ChangeExec     = "exec"
	ChangeNodeType = "node_type"
)

// TextDiff is the unified rendering of one content change. Empty when the
// change does not qualify for text rendering (binary, undecodable, or
// beyond the presentation limits).
type TextDiff struct {
	Unified string `json:"unified"`
}

// Entry is one changed path between two trees. Changes carries at least one
// kind; add/delete have only the existing side, node_type replaces every
// other kind, and content carries the optional unified text.
type Entry struct {
	Path    string    `json:"path"`
	Changes []string  `json:"changes"`
	From    *Node     `json:"from,omitempty"`
	To      *Node     `json:"to,omitempty"`
	Text    *TextDiff `json:"text,omitempty"`
}

// Comparison is one of the three exposed pairwise differences with its
// side labels.
type Comparison struct {
	From    string  `json:"from"`
	To      string  `json:"to"`
	Entries []Entry `json:"entries"`
}

// ThreeWay computes the decision-05 comparisons in order: baseline to
// source, baseline to store, and source to store.
func ThreeWay(baseline, source, store Tree) []Comparison {
	return []Comparison{
		Compare("baseline", "source", baseline, source),
		Compare("baseline", "store", baseline, store),
		Compare("source", "store", source, store),
	}
}

// Compare diffs two trees path by path. From/To are labels, not content;
// a change is classified relative to the target side (to): add means only
// the target carries the path, delete only the source.
func Compare(from, to string, a, b Tree) Comparison {
	c := Comparison{From: from, To: to, Entries: []Entry{}}
	paths := map[string]bool{}
	for p := range a {
		paths[p] = true
	}
	for p := range b {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	for _, p := range sorted {
		na, inA := a[p]
		nb, inB := b[p]
		e := Entry{Path: p}
		switch {
		case !inA:
			e.Changes = []string{ChangeAdd}
			e.To = &nb
			if nb.Kind == KindFile && nb.Text != nil {
				e.Text = &TextDiff{Unified: UnifiedText("", *nb.Text)}
			}
		case !inB:
			e.Changes = []string{ChangeDelete}
			e.From = &na
			if na.Kind == KindFile && na.Text != nil {
				e.Text = &TextDiff{Unified: UnifiedText(*na.Text, "")}
			}
		default:
			e.From, e.To = &na, &nb
			switch {
			case na.Kind != nb.Kind:
				e.Changes = []string{ChangeNodeType}
			default:
				if na.Digest != nb.Digest {
					e.Changes = append(e.Changes, ChangeContent)
					if na.Kind == KindFile && na.Text != nil && nb.Text != nil {
						e.Text = &TextDiff{Unified: UnifiedText(*na.Text, *nb.Text)}
					}
				}
				if execChanged(na, nb) {
					e.Changes = append(e.Changes, ChangeExec)
				}
			}
		}
		if len(e.Changes) == 0 {
			continue
		}
		c.Entries = append(c.Entries, e)
	}
	return c
}

// execChanged reports an executable-bit difference for file or directory
// nodes that carry one.
func execChanged(a, b Node) bool {
	if a.Exec == nil || b.Exec == nil {
		return false
	}
	return *a.Exec != *b.Exec
}

// FilterEntries keeps only entries at or below the given path.
func (c Comparison) FilterEntries(path string) Comparison {
	if path == "" || path == "." {
		return c
	}
	prefix := path + "/"
	out := Comparison{From: c.From, To: c.To, Entries: []Entry{}}
	for _, e := range c.Entries {
		if e.Path == path || strings.HasPrefix(e.Path, prefix) {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
}
