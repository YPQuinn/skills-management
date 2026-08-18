package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// NewSkillCmd builds the noun-first `skillctl skill` command group.
func NewSkillCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage Skills in the Store",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		NewSkillListCmd(bm),
		NewSkillShowCmd(bm),
		NewSkillImportCmd(bm),
		NewSkillCheckSyncCmd(bm),
		NewSkillDiffCmd(bm),
		NewSkillSyncCmd(bm),
		NewSkillKeepStoreCmd(bm),
		NewSkillAcceptSourceCmd(bm),
		NewSkillRollbackCmd(bm),
		NewSkillDetachCmd(bm),
		NewSkillRebindCmd(bm),
		NewSkillDeleteCmd(bm),
	)
	return cmd
}

func NewSkillListCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Skills in the Store",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			items, err := a.ListSkills()
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, map[string]any{"items": items, "total": len(items)})
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Skills.")
				return nil
			}
			for _, s := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\n", s.ID, s.Slug, s.Name, skillBindingLabel(s))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d Skill(s)\n", len(items))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSkillShowCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <slug-or-id>",
		Short: "Show one Skill and its Source Binding",
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
			s, err := a.ShowSkill(id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, s)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Skill %d: %s\n", s.ID, s.Slug)
			fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\nDescription: %s\n", s.Name, s.Description)
			fmt.Fprintf(cmd.OutOrStdout(), "Store digest: %s\nBaseline digest: %s\n", s.StoreDigest, s.BaselineDigest)
			fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\nUpdated: %s\n", formatHumanTime(&s.CreatedAt), formatHumanTime(&s.UpdatedAt))
			fmt.Fprintf(cmd.OutOrStdout(), "Sync status: %s", s.SyncStatus)
			if s.SyncStale {
				fmt.Fprint(cmd.OutOrStdout(), " (stale)")
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if s.SyncCheckedAt != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Sync checked: %s\n", formatHumanTime(s.SyncCheckedAt))
			}
			if s.LastSync != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Last sync: %s -> %s", s.LastSync.Action, s.LastSync.Result)
				if s.LastSync.Error != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " (%s)", s.LastSync.Error)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			if s.Binding == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Binding: unbound")
				return nil
			}
			b := s.Binding
			fmt.Fprintf(cmd.OutOrStdout(), "Binding: Source %q (Source %d)\n", b.SourceName, b.SourceID)
			fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\nDigest: %s\n", b.RelativeDir, b.Digest)
			if b.SourceCommit != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Commit: %s\n", b.SourceCommit)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported: %s\n", formatHumanTime(&b.ImportedAt))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSkillImportCmd(bm *bootstrap.Manager) *cobra.Command {
	var sourceArg string
	var paths, skillNames []string
	var all, allowLarge, replace bool
	var slug string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import Skills from one Source into the Store",
		Long: `Import selected Inventory entries from one Source into the Skill Store.
Exactly one of --all or at least one --path/--skill selector is required.
--slug and --replace are single-selector options and are forbidden with
--all. A batch is per-Skill: every entry reports its own outcome and one
failure never rolls back its siblings.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The flag contract is validated before the installation is
			// touched, so contract errors never depend on state. An explicit
			// --slug= is distinguishable from an omitted --slug, so the
			// empty value reaches the application's slug validation.
			slugSet := cmd.Flags().Changed("slug")
			if err := validateImportFlags(sourceArg, paths, skillNames, all, slugSet, replace); err != nil {
				return err
			}
			a, err := bm.App()
			if err != nil {
				return err
			}
			sourceID, err := a.ResolveSourceArg(sourceArg)
			if err != nil {
				return err
			}
			in := buildImportInput(sourceID, paths, skillNames, all, slug, slugSet, replace, allowLarge)
			result, err := a.ImportSkills(cmd.Context(), in)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, newImportResultView(result))
			}
			printImportHuman(cmd, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceArg, "source", "", "Source name or id (required)")
	cmd.Flags().StringArrayVar(&paths, "path", nil, "Source Inventory entry by relative path (repeatable)")
	cmd.Flags().StringArrayVar(&skillNames, "skill", nil, "Source Inventory entry by unique name (repeatable)")
	cmd.Flags().BoolVar(&all, "all", false, "import every Inventory entry")
	cmd.Flags().StringVar(&slug, "slug", "", "explicit slug for a single selector")
	cmd.Flags().BoolVar(&replace, "replace", false, "replace the existing Skill claiming the final slug")
	cmd.Flags().BoolVar(&allowLarge, "allow-large", false, "permit Skills beyond the default size and file-count guards")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// validateImportFlags enforces the strict CLI flag contract before any
// application call: --source is required, exactly one of --all or a
// non-empty selector list must be given, --all forbids selectors, and
// --slug/--replace are single-selector options forbidden with --all.
// slugSet is whether --slug was explicitly provided, so an explicit empty
// value still counts as provided. Per-selector path-or-name choice, slug
// grammar, and duplicate detection are enforced by the application.
func validateImportFlags(sourceArg string, paths, skillNames []string, all, slugSet, replace bool) error {
	switch {
	case sourceArg == "":
		return app.Errorf(app.CodeInvalidArgument, "--source is required")
	case all && len(paths)+len(skillNames) > 0:
		return app.Errorf(app.CodeInvalidArgument, "selectors cannot be combined with --all")
	case all && (slugSet || replace):
		return app.Errorf(app.CodeInvalidArgument, "--slug and --replace cannot be combined with --all")
	case !all && len(paths)+len(skillNames) == 0:
		return app.Errorf(app.CodeInvalidArgument, "select at least one --path or --skill, or --all")
	case (slugSet || replace) && len(paths)+len(skillNames) != 1:
		return app.Errorf(app.CodeInvalidArgument, "--slug and --replace require exactly one selector")
	}
	return nil
}

// buildImportInput maps the validated flag contract onto the application
// request. Callers must have run validateImportFlags first. When --slug was
// explicitly provided (slugSet), the exact value is passed as a pointer so
// the application's slug validation also rejects an explicit empty slug;
// an omitted --slug leaves the pointer nil.
func buildImportInput(sourceID int64, paths, skillNames []string, all bool, slug string, slugSet, replace, allowLarge bool) app.ImportSkillsInput {
	selectors := make([]app.ImportSelector, 0, len(paths)+len(skillNames))
	for _, p := range paths {
		selectors = append(selectors, app.ImportSelector{RelativeDir: p})
	}
	for _, n := range skillNames {
		selectors = append(selectors, app.ImportSelector{Name: n})
	}
	if slugSet {
		selectors[0].Slug = &slug
	}
	if replace {
		selectors[0].Replace = true
	}
	return app.ImportSkillsInput{SourceID: sourceID, Selectors: selectors, All: all, AllowLarge: allowLarge}
}

// printImportHuman prints every item outcome followed by the batch summary.
func printImportHuman(cmd *cobra.Command, result *app.ImportSkillsResult) {
	for _, it := range result.Items {
		switch it.Status {
		case app.StatusImported:
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %s as %q (Skill %d)\n", it.RelativeDir, it.Slug, it.SkillID)
		case app.StatusAlreadyImported:
			fmt.Fprintf(cmd.OutOrStdout(), "Already imported: %s -> %q (Skill %d)\n", it.RelativeDir, it.Slug, it.SkillID)
		case app.StatusSkippedConflict:
			fmt.Fprintf(cmd.OutOrStdout(), "Skipped conflict: %s would claim %q owned by Skill %d\n", it.RelativeDir, it.Slug, it.SkillID)
		case app.StatusReplaced:
			fmt.Fprintf(cmd.OutOrStdout(), "Replaced Skill %d with %s (%q)\n", it.SkillID, it.RelativeDir, it.Slug)
		case app.StatusFailed:
			fmt.Fprintf(cmd.OutOrStdout(), "Failed %s: [%s] %s\n", it.RelativeDir, it.ErrorCode, it.ErrorMessage)
		}
	}
	s := result.Summary
	fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d total, %d imported, %d already imported, %d skipped conflicts, %d replaced, %d failed\n",
		s.Total, s.Imported, s.AlreadyImported, s.SkippedConflict, s.Replaced, s.Failed)
}

// skillBindingLabel renders one Skill's binding column for list output.
func skillBindingLabel(s app.Skill) string {
	if s.Binding == nil {
		return "unbound"
	}
	return fmt.Sprintf("Source %q (%s)", s.Binding.SourceName, s.Binding.RelativeDir)
}
