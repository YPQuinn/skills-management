package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// syncSummaryJSONView mirrors the REST synchronization batch summary.
type syncSummaryJSONView struct {
	Total          int `json:"total"`
	NoOp           int `json:"no_op"`
	Updated        int `json:"updated"`
	KeptStore      int `json:"kept_store"`
	AcceptedSource int `json:"accepted_source"`
	Skipped        int `json:"skipped"`
	Blocked        int `json:"blocked"`
	Failed         int `json:"failed"`
	RolledBack     int `json:"rolled_back"`
}

// syncBatchJSONView is the --json shape of one Source synchronization batch:
// the item results plus the summary, matching the REST response.
type syncBatchJSONView struct {
	Items   []syncItemJSONView  `json:"items"`
	Summary syncSummaryJSONView `json:"summary"`
}

func newSyncBatchView(r *app.SyncSkillsResult) syncBatchJSONView {
	out := syncBatchJSONView{
		Items: make([]syncItemJSONView, 0, len(r.Items)),
		Summary: syncSummaryJSONView{
			Total: r.Summary.Total, NoOp: r.Summary.NoOp, Updated: r.Summary.Updated,
			KeptStore: r.Summary.KeptStore, AcceptedSource: r.Summary.AcceptedSource,
			Skipped: r.Summary.Skipped, Blocked: r.Summary.Blocked,
			Failed: r.Summary.Failed, RolledBack: r.Summary.RolledBack,
		},
	}
	for _, it := range r.Items {
		out.Items = append(out.Items, newSyncItemView(&it))
	}
	return out
}

// NewSourceSyncCmd synchronizes every Skill bound to one Source as one
// batch under a single fresh coherent observation.
func NewSourceSyncCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync <name-or-id>",
		Short: "Synchronize every Skill bound to one Source (one fresh check)",
		Long: `Synchronize every Skill bound to one Source with a single fresh coherent
Source check. Each Skill then applies the safe synchronization rules on its
own: source_changed updates automatically, store_changed and conflict are
skipped until an explicit action, and missing or invalid entries are
blocked. One Skill's failure never rolls back another's success.`,
		Example: `  skillctl source sync local`,
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			sourceID, err := a.ResolveSourceArg(args[0])
			if err != nil {
				return err
			}
			result, err := a.SyncSkills(cmd.Context(), sourceID)
			if err != nil {
				return err
			}
			var partial error
			for _, it := range result.Items {
				if it.Result == "skipped" || it.Result == "blocked" || it.Result == "failed" {
					partial = exitPartial("%d item(s) were skipped, blocked, or failed", result.Summary.Skipped+result.Summary.Blocked+result.Summary.Failed)
				}
			}
			if jsonRequested {
				if err := printJSON(cmd, newSyncBatchView(result)); err != nil {
					return err
				}
				return partial
			}
			printSyncBatchHuman(cmd, result)
			return partial
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// printSyncBatchHuman prints every item outcome followed by the batch
// summary.
func printSyncBatchHuman(cmd *cobra.Command, result *app.SyncSkillsResult) {
	for _, it := range result.Items {
		switch it.Result {
		case "updated":
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %q from %s\n", it.Slug, it.Revision)
		case "no_op":
			fmt.Fprintf(cmd.OutOrStdout(), "In sync: %q\n", it.Slug)
		case "skipped":
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped %q: %s requires an explicit action\n", it.Slug, it.Status)
		case "blocked":
			fmt.Fprintf(cmd.OutOrStdout(), "Blocked %q: %s\n", it.Slug, it.ErrorMessage)
		case "failed":
			fmt.Fprintf(cmd.OutOrStdout(), "Failed %q: [%s] %s\n", it.Slug, it.ErrorCode, it.ErrorMessage)
		}
	}
	s := result.Summary
	fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d total, %d in sync, %d updated, %d skipped, %d blocked, %d failed\n",
		s.Total, s.NoOp, s.Updated, s.Skipped, s.Blocked, s.Failed)
}

// printDiffHuman prints the three comparisons with their unified text.
func printDiffHuman(cmd *cobra.Command, r *app.DiffResult) {
	fmt.Fprintf(cmd.OutOrStdout(), "Skill %q: source %s, store %s, baseline %s\n",
		r.Slug, shortDigest(r.SourceDigest), shortDigest(r.StoreDigest), shortDigest(r.BaselineDigest))
	for _, c := range r.Comparisons {
		fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", c.From, c.To)
		if len(c.Entries) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "  (identical)")
			continue
		}
		for _, e := range c.Entries {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s %s", e.Path, joinChanges(e.Changes))
			if e.From != nil && e.To != nil {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s %s -> %s %s)", e.From.Kind, shortDigest(e.From.Digest), e.To.Kind, shortDigest(e.To.Digest))
			} else if e.From != nil {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s %s)", e.From.Kind, shortDigest(e.From.Digest))
			} else if e.To != nil {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s %s)", e.To.Kind, shortDigest(e.To.Digest))
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if e.Text != nil && e.Text.Unified != "" {
				fmt.Fprint(cmd.OutOrStdout(), e.Text.Unified)
			}
		}
	}
}

func joinChanges(changes []string) string {
	out := ""
	for i, c := range changes {
		if i > 0 {
			out += "+"
		}
		out += c
	}
	return out
}

func shortDigest(d string) string {
	if len(d) > 8 {
		return d[:8]
	}
	if d == "" {
		return "-"
	}
	return d
}
