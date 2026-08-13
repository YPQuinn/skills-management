package app

// ImportStatus is the stable per-item outcome of one batch import. These
// exact values are the machine-consumed contract shared by CLI, REST, and
// WebUI.
type ImportStatus string

const (
	// StatusImported means the entry was imported into the Skill Store.
	StatusImported ImportStatus = "imported"
	// StatusAlreadyImported means the same Source entry was already bound
	// to a Skill, so the item was a no-op.
	StatusAlreadyImported ImportStatus = "already_imported"
	// StatusSkippedConflict means the final slug was claimed by a different
	// Skill and no explicit Replace was requested.
	StatusSkippedConflict ImportStatus = "skipped_conflict"
	// StatusReplaced means an existing managed Skill was explicitly
	// replaced, retaining its id and slug.
	StatusReplaced ImportStatus = "replaced"
	// StatusFailed means the item could not be imported; the ErrorCode and
	// ErrorMessage describe the stable failure.
	StatusFailed ImportStatus = "failed"
)

// ImportItemResult is one batch item's stable outcome. RelativeDir
// identifies the Source Inventory entry; RequestedSlug is the explicit
// override or derived default the operator asked for, Slug is the final
// slug the item resolved to, and SkillID is the committed Skill id when the
// item produced or affected one. A failed item carries a stable ErrorCode
// and message. Replaces previews the existing managed Skill and its Source
// Binding that an explicit Replace would supersede.
type ImportItemResult struct {
	Status        ImportStatus `json:"status"`
	RelativeDir   string       `json:"relative_dir"`
	RequestedSlug string       `json:"requested_slug,omitempty"`
	Slug          string       `json:"slug,omitempty"`
	SkillID       int64        `json:"skill_id,omitempty"`
	ErrorCode     string       `json:"error_code,omitempty"`
	ErrorMessage  string       `json:"error_message,omitempty"`
	Replaces      *Skill       `json:"replaces,omitempty"`
	// Impact previews the Groups and Targets affected when an explicit
	// Replace supersedes the existing Skill (ticket 13).
	Impact *ReplaceImpact `json:"impact,omitempty"`
}

// ImportSkillsSummary is the batch-level tally of per-item statuses.
type ImportSkillsSummary struct {
	Total           int `json:"total"`
	Imported        int `json:"imported"`
	AlreadyImported int `json:"already_imported"`
	SkippedConflict int `json:"skipped_conflict"`
	Replaced        int `json:"replaced"`
	Failed          int `json:"failed"`
}

// ImportSkillsResult is the complete outcome of one valid batch: every item
// result plus the summary. A batch that began processing returns item
// outcomes rather than a top-level error.
type ImportSkillsResult struct {
	Items   []ImportItemResult  `json:"items"`
	Summary ImportSkillsSummary `json:"summary"`
}

// SummarizeImportItems tallies one batch's item outcomes into the batch
// summary. An unknown status still counts toward Total but no per-status
// bucket, so a summary never silently hides an item.
func SummarizeImportItems(items []ImportItemResult) ImportSkillsSummary {
	var s ImportSkillsSummary
	s.Total = len(items)
	for _, it := range items {
		switch it.Status {
		case StatusImported:
			s.Imported++
		case StatusAlreadyImported:
			s.AlreadyImported++
		case StatusSkippedConflict:
			s.SkippedConflict++
		case StatusReplaced:
			s.Replaced++
		case StatusFailed:
			s.Failed++
		}
	}
	return s
}
