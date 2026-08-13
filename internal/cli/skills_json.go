package cli

import "skillctl/internal/app"

// The CLI --json import result view mirrors the frozen REST import response
// exactly (decision 08: CLI --json preserves the same result fields without
// the HTTP envelope): snake_case statuses, relative_dir, requested_slug,
// slug, skill_id, optional code/message, compact replaces, and the six-field
// summary. It is deliberately narrow and independent of internal/httpapi;
// the human output does not use it.

// importReplacesView previews the existing managed Skill an explicit
// Replace would supersede.
type importReplacesView struct {
	SkillID int64  `json:"skill_id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

// importImpactGroupView is one affected Group in the replace-impact
// preview.
type importImpactGroupView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// importImpactTargetView is one affected Target with how the Skill reaches
// it.
type importImpactTargetView struct {
	ID     int64                   `json:"id"`
	Name   string                  `json:"name"`
	Direct bool                    `json:"direct"`
	Groups []importImpactGroupView `json:"groups,omitempty"`
}

// importImpactView previews the Groups and Targets affected by an explicit
// Replace.
type importImpactView struct {
	Groups  []importImpactGroupView  `json:"groups"`
	Targets []importImpactTargetView `json:"targets"`
}

// importItemView is one batch item's stable outcome. Code and Message are
// present only for failed items; Replaces and Impact only for conflict and
// replaced items.
type importItemView struct {
	Status        string              `json:"status"`
	RelativeDir   string              `json:"relative_dir"`
	RequestedSlug string              `json:"requested_slug,omitempty"`
	Slug          string              `json:"slug,omitempty"`
	SkillID       int64               `json:"skill_id,omitempty"`
	Code          string              `json:"code,omitempty"`
	Message       string              `json:"message,omitempty"`
	Replaces      *importReplacesView `json:"replaces,omitempty"`
	Impact        *importImpactView   `json:"impact,omitempty"`
}

// importSummaryView tallies the per-item statuses of one batch.
type importSummaryView struct {
	Total           int `json:"total"`
	Imported        int `json:"imported"`
	AlreadyImported int `json:"already_imported"`
	SkippedConflict int `json:"skipped_conflict"`
	Replaced        int `json:"replaced"`
	Failed          int `json:"failed"`
}

// importResultView is the complete outcome of one valid batch.
type importResultView struct {
	Items   []importItemView  `json:"items"`
	Summary importSummaryView `json:"summary"`
}

// newImportResultView maps the application result onto the frozen JSON
// shape. Replaces keeps only the identity the replace flow needs.
func newImportResultView(r *app.ImportSkillsResult) importResultView {
	out := importResultView{
		Items: make([]importItemView, 0, len(r.Items)),
		Summary: importSummaryView{
			Total: r.Summary.Total, Imported: r.Summary.Imported,
			AlreadyImported: r.Summary.AlreadyImported, SkippedConflict: r.Summary.SkippedConflict,
			Replaced: r.Summary.Replaced, Failed: r.Summary.Failed,
		},
	}
	for _, it := range r.Items {
		item := importItemView{
			Status: string(it.Status), RelativeDir: it.RelativeDir,
			RequestedSlug: it.RequestedSlug, Slug: it.Slug, SkillID: it.SkillID,
			Code: it.ErrorCode, Message: it.ErrorMessage,
		}
		if it.Replaces != nil {
			item.Replaces = &importReplacesView{
				SkillID: it.Replaces.ID, Slug: it.Replaces.Slug, Name: it.Replaces.Name,
			}
		}
		if it.Impact != nil {
			item.Impact = &importImpactView{
				Groups:  make([]importImpactGroupView, 0, len(it.Impact.Groups)),
				Targets: make([]importImpactTargetView, 0, len(it.Impact.Targets)),
			}
			for _, g := range it.Impact.Groups {
				item.Impact.Groups = append(item.Impact.Groups, importImpactGroupView{ID: g.ID, Name: g.Name})
			}
			for _, tg := range it.Impact.Targets {
				t := importImpactTargetView{ID: tg.ID, Name: tg.Name, Direct: tg.Direct, Groups: []importImpactGroupView{}}
				for _, g := range tg.Groups {
					t.Groups = append(t.Groups, importImpactGroupView{ID: g.ID, Name: g.Name})
				}
				item.Impact.Targets = append(item.Impact.Targets, t)
			}
		}
		out.Items = append(out.Items, item)
	}
	return out
}
