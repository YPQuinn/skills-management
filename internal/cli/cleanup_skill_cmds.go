package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

func NewSourceDeleteCmd(bm *bootstrap.Manager) *cobra.Command {
	var detach, yes bool
	cmd := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a Source, optionally detaching bound Skills",
		Long: `Delete one Source. Bound Skills block deletion unless --detach-skills
is set; those Skills are detached and never deleted.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSourceArg(args[0])
			if err != nil {
				return err
			}
			preview, err := a.PreviewDeleteSource(id)
			if err != nil {
				return err
			}
			if !jsonRequested {
				printSourceDeletePreview(cmd, preview)
			}
			if err := confirmOrYes(cmd, yes, fmt.Sprintf("Delete Source %q?", preview.Name)); err != nil {
				return err
			}
			res, err := a.DeleteSource(cmd.Context(), id, detach)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted Source %q\n", res.Name)
			if len(res.Detached) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Detached %d Skill(s): %s\n", len(res.Detached), skillRefsCSV(res.Detached))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&detach, "detach-skills", false, "detach bound Skills instead of blocking")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSkillDetachCmd(bm *bootstrap.Manager) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "detach <slug-or-id>",
		Short: "Remove a Skill's Source Binding without deleting the Skill",
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
			sk, err := a.ShowSkill(id)
			if err != nil {
				return err
			}
			if !jsonRequested {
				if sk.Binding == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Skill %q is already unbound\n", sk.Slug)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Detach Skill %q from Source %q (%s)\n",
						sk.Slug, sk.Binding.SourceName, sk.Binding.RelativeDir)
				}
			}
			if err := confirmOrYes(cmd, yes, fmt.Sprintf("Detach Skill %q?", sk.Slug)); err != nil {
				return err
			}
			out, err := a.DetachSkill(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, out)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Detached Skill %q (now unbound)\n", out.Slug)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSkillRebindCmd(bm *bootstrap.Manager) *cobra.Command {
	var sourceArg, path, skillName string
	var yes bool
	cmd := &cobra.Command{
		Use:   "rebind <slug-or-id>",
		Short: "Bind a Skill to an explicit Source Inventory entry",
		Long: `Bind one Skill to one Source Inventory entry. Identical content
becomes in_sync. Different content enters a synchronization conflict and
never overwrites the Store.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if path != "" && skillName != "" {
				return app.Errorf(app.CodeInvalidArgument, "choose either --path or --skill, not both")
			}
			if path == "" && skillName == "" {
				return app.Errorf(app.CodeInvalidArgument, "--path or --skill is required")
			}
			if sourceArg == "" {
				return app.Errorf(app.CodeInvalidArgument, "--source is required")
			}
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			sourceID, err := a.ResolveSourceArg(sourceArg)
			if err != nil {
				return err
			}
			if !jsonRequested {
				sel := path
				if sel == "" {
					sel = skillName
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Rebind Skill %s to Source %s entry %s\n", args[0], sourceArg, sel)
			}
			if err := confirmOrYes(cmd, yes, "Rebind this Skill?"); err != nil {
				return err
			}
			res, err := a.RebindSkill(cmd.Context(), id, sourceID, path, skillName)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, res)
			}
			switch res.Skill.SyncStatus {
			case "in_sync":
				fmt.Fprintf(cmd.OutOrStdout(), "Rebound Skill %q (in_sync)\n", res.Skill.Slug)
			case "conflict":
				fmt.Fprintf(cmd.OutOrStdout(), "Rebound Skill %q (sync conflict; Store content was not overwritten)\n", res.Skill.Slug)
			default:
				fmt.Fprintf(cmd.OutOrStdout(), "Rebound Skill %q (%s)\n", res.Skill.Slug, res.Skill.SyncStatus)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceArg, "source", "", "Source name or id")
	cmd.Flags().StringVar(&path, "path", "", "Inventory relative directory")
	cmd.Flags().StringVar(&skillName, "skill", "", "unique Inventory name")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewSkillDeleteCmd(bm *bootstrap.Manager) *cobra.Command {
	var cleanup, yes bool
	cmd := &cobra.Command{
		Use:   "delete <slug-or-id>",
		Short: "Delete a Skill from the Store",
		Long: `Delete one Skill. A referenced Skill is blocked unless --cleanup
is set, which first removes related Assignments and verifiable Managed
Links. Ownership-lost Target paths are left untouched.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveSkillArg(args[0])
			if err != nil {
				return err
			}
			preview, err := a.PreviewDeleteSkill(id)
			if err != nil {
				return err
			}
			if !jsonRequested {
				printSkillDeletePreview(cmd, preview)
			}
			if err := confirmOrYes(cmd, yes, fmt.Sprintf("Delete Skill %q?", preview.Skill.Slug)); err != nil {
				return err
			}
			res, err := a.DeleteSkill(cmd.Context(), id, cleanup)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted Skill %q\n", res.Skill.Slug)
			printCleanupLinks(cmd, res.Links)
			return nil
		},
	}
	cmd.Flags().BoolVar(&cleanup, "cleanup", false, "remove related Assignments and Managed Links first")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}
