package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/bootstrap"
)

func NewTargetDeleteCmd(bm *bootstrap.Manager) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a Target registration after cleaning Managed Links",
		Long: `Remove every verifiable Managed Link and then the Target
registration. The Target container is never deleted.`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveTargetArg(args[0])
			if err != nil {
				return err
			}
			preview, err := a.PreviewDeleteTarget(id)
			if err != nil {
				return err
			}
			if !jsonRequested {
				printTargetDeletePreview(cmd, preview)
			}
			if err := confirmOrYes(cmd, yes, fmt.Sprintf("Delete Target %q?", preview.Target.Name)); err != nil {
				return err
			}
			res, err := a.DeleteTarget(cmd.Context(), id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted Target %q (container kept)\n", res.Target.Name)
			printCleanupLinks(cmd, res.Links)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewGroupDeleteCmd(bm *bootstrap.Manager) *cobra.Command {
	var unassign, yes bool
	cmd := &cobra.Command{
		Use:   "delete <name-or-id>",
		Short: "Delete a Group, optionally unassigning it from Targets",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveGroupArg(args[0])
			if err != nil {
				return err
			}
			preview, err := a.PreviewDeleteGroup(id)
			if err != nil {
				return err
			}
			if !jsonRequested {
				printGroupDeletePreview(cmd, preview)
			}
			if err := confirmOrYes(cmd, yes, fmt.Sprintf("Delete Group %q?", preview.Group.Name)); err != nil {
				return err
			}
			res, err := a.DeleteGroup(cmd.Context(), id, unassign)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted Group %q\n", res.Group.Name)
			if len(res.Unassigned) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Unassigned from %d Target(s)\n", len(res.Unassigned))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&unassign, "unassign", false, "remove Group Assignments instead of blocking")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}
