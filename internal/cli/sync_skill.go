package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// syncItemJSONView mirrors the REST per-item synchronization outcome for
// --json output (decision 08: the same result fields without the HTTP
// envelope).
type syncItemJSONView struct {
	SkillID      int64  `json:"skill_id"`
	Slug         string `json:"slug"`
	Status       string `json:"status"`
	Stale        bool   `json:"stale"`
	Action       string `json:"action"`
	Result       string `json:"result"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"message,omitempty"`
	BeforeDigest string `json:"before_digest,omitempty"`
	AfterDigest  string `json:"after_digest,omitempty"`
	Revision     string `json:"revision,omitempty"`
}

func newSyncItemView(r *app.SyncItemResult) syncItemJSONView {
	return syncItemJSONView{
		SkillID: r.SkillID, Slug: r.Slug, Status: string(r.Status), Stale: r.Stale,
		Action: r.Action, Result: r.Result, Code: r.ErrorCode, Message: r.ErrorMessage,
		BeforeDigest: r.BeforeDigest, AfterDigest: r.AfterDigest, Revision: r.Revision,
	}
}

// syncBlockedExit maps one reported outcome onto the process result: a
// blocked, skipped, or failed use case already printed its complete result
// and exits 1.
func syncBlockedExit(r *app.SyncItemResult) error {
	switch r.Result {
	case "skipped", "blocked", "failed":
		return exitPartial("%s: %s %s", r.Slug, r.Result, r.ErrorMessage)
	}
	return nil
}

// NewSkillCheckSyncCmd re-observes one Skill's Source and persists the
// evaluated Sync Status.
func NewSkillCheckSyncCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check <slug-or-id>",
		Short: "Re-observe one Skill's Source and evaluate its Sync Status",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			s, err := a.CheckSkillSync(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, s)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Skill %q: %s", s.Slug, s.SyncStatus)
			if s.SyncStale {
				fmt.Fprint(cmd.OutOrStdout(), " (stale)")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewSkillDiffCmd shows the three-way difference of one bound Skill.
func NewSkillDiffCmd(bm *bootstrap.Manager) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "diff <slug-or-id>",
		Short: "Show the three-way difference between Baseline, Source, and Store",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			r, err := a.DiffSkill(cmd.Context(), id, path)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, r)
			}
			printDiffHuman(cmd, r)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "show only one path and its descendants")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewSkillSyncCmd runs the safe synchronization of one Skill.
func NewSkillSyncCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync <slug-or-id>",
		Short: "Synchronize one Skill from its Source (source_changed only)",
		Long: `Refresh the bound Source and apply the safe synchronization rules.
source_changed updates Store content automatically. in_sync is a no-op.
store_changed and conflict are skipped until keep-store or accept-source.
source_missing, source_invalid, and an unavailable Source block retrieval.`,
		Example: `  skillctl skill sync demo-skill`,
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			r, err := a.SyncSkill(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				if err := printJSON(cmd, newSyncItemView(r)); err != nil {
					return err
				}
				return syncBlockedExit(r)
			}
			printSyncItemHuman(cmd, r)
			return syncBlockedExit(r)
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewSkillKeepStoreCmd accepts the observed Source as the new Baseline.
func NewSkillKeepStoreCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keep-store <slug-or-id>",
		Short: "Keep Store content and accept the Source as the new Baseline",
		Long: `Leave live Store content untouched and accept the currently
observed Source as the new Synchronization Baseline. Use this for
store_changed or conflict. Ordinary sync skips those states and never
implies keep-store.`,
		Example: `  skillctl skill keep-store demo-skill`,
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			r, err := a.KeepStore(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				if err := printJSON(cmd, newSyncItemView(r)); err != nil {
					return err
				}
				return syncBlockedExit(r)
			}
			printSyncItemHuman(cmd, r)
			return syncBlockedExit(r)
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewSkillAcceptSourceCmd replaces Store content with validated Source
// content.
func NewSkillAcceptSourceCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accept-source <slug-or-id>",
		Short: "Replace Store content with the Source content (explicit)",
		Long: `Snapshot current Store content, then replace it with validated
Source content and advance the Baseline. Use this for store_changed,
conflict, store_missing, or store_invalid. Ordinary sync never implies
accept-source.`,
		Example: `  skillctl skill accept-source demo-skill`,
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			r, err := a.AcceptSource(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				if err := printJSON(cmd, newSyncItemView(r)); err != nil {
					return err
				}
				return syncBlockedExit(r)
			}
			printSyncItemHuman(cmd, r)
			return syncBlockedExit(r)
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewSkillRollbackCmd restores the single previous snapshot.
func NewSkillRollbackCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback <slug-or-id>",
		Short: "Roll back one Skill to its previous snapshot",
		Long: `Restore the single previous Store snapshot. The displaced live
tree becomes the new previous snapshot, so a second rollback undoes the
first. Binding, Group, and Assignment relationships are unchanged.`,
		Example: `  skillctl skill rollback demo-skill`,
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			r, err := a.Rollback(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				if err := printJSON(cmd, newSyncItemView(r)); err != nil {
					return err
				}
				return syncBlockedExit(r)
			}
			printSyncItemHuman(cmd, r)
			return syncBlockedExit(r)
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// printSyncItemHuman prints one single-skill action outcome.
func printSyncItemHuman(cmd *cobra.Command, r *app.SyncItemResult) {
	switch r.Result {
	case "failed", "blocked":
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q: [%s] %s\n", r.Result, r.Slug, r.ErrorCode, r.ErrorMessage)
	case "skipped":
		fmt.Fprintf(cmd.OutOrStdout(), "Skipped %q: status %s requires an explicit action (keep-store or accept-source)\n", r.Slug, r.Status)
	default:
		fmt.Fprintf(cmd.OutOrStdout(), "%s %q (status %s)\n", r.Result, r.Slug, r.Status)
	}
}
