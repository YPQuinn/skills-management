package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// NewTargetCmd builds the noun-first `skillctl target` command group.
func NewTargetCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "target",
		Short: "Manage Agent Targets",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		NewTargetAddCmd(bm),
		NewTargetListCmd(bm),
		NewTargetShowCmd(bm),
		NewTargetStatusCmd(bm),
		NewTargetDistributeCmd(bm),
		NewTargetAdoptCmd(bm),
		NewTargetAssignCmd(bm),
		NewTargetUnassignCmd(bm),
		NewTargetDeleteCmd(bm),
	)
	return cmd
}

func NewTargetAddCmd(bm *bootstrap.Manager) *cobra.Command {
	var adapter, scope, project, path, name string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Register a Target",
		Long: `Register one Target. A built-in adapter form is
--adapter <adapter> --scope user
--adapter <adapter> --scope project --project <root>
and a custom container form is --path <dir>. Registration is read-only and
returns the existing Target when the resolved path is already registered.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The flag contract is validated before the installation is
			// touched; an explicitly set empty value still counts as set.
			pathSet := cmd.Flags().Changed("path")
			adapterSet := cmd.Flags().Changed("adapter")
			scopeSet := cmd.Flags().Changed("scope")
			projectSet := cmd.Flags().Changed("project")
			if pathSet == adapterSet {
				return app.Errorf(app.CodeInvalidArgument, "exactly one of --path or --adapter is required")
			}
			if pathSet {
				if scopeSet || projectSet {
					return app.Errorf(app.CodeInvalidArgument, "--scope and --project cannot be combined with --path")
				}
			} else if !scopeSet || (scope != "user" && scope != "project") {
				return app.Errorf(app.CodeInvalidArgument, "--scope must be %q or %q", "user", "project")
			} else if scope == "project" && !projectSet {
				return app.Errorf(app.CodeInvalidArgument, "--project is required for a project Target")
			} else if scope == "user" && projectSet {
				return app.Errorf(app.CodeInvalidArgument, "--project cannot be combined with --scope user")
			}
			a, err := bm.App()
			if err != nil {
				return err
			}
			view, err := a.RegisterTarget(app.TargetInput{
				Name: name, Adapter: adapter, Scope: scope, ProjectRoot: project, Path: path,
			})
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, view)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered Target %d %q -> %s\n", view.ID, view.Name, view.Path)
			fmt.Fprintf(cmd.OutOrStdout(), "Adapter: %s (scope %s)\n", view.Adapter, view.Scope)
			return nil
		},
	}
	cmd.Flags().StringVar(&adapter, "adapter", "", "built-in Target Adapter key")
	cmd.Flags().StringVar(&scope, "scope", "", "user or project")
	cmd.Flags().StringVar(&project, "project", "", "project root directory (project scope)")
	cmd.Flags().StringVar(&path, "path", "", "custom Skills container path (absolute or ~/...)")
	cmd.Flags().StringVar(&name, "name", "", "operator-facing Target name")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewTargetListCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered Targets",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			items, err := a.ListTargets()
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, map[string]any{"items": items, "total": len(items)})
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Targets.")
				return nil
			}
			for _, t := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\t%s\t%s\n", t.ID, t.Name, t.Adapter, t.Scope, t.Path)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d Target(s)\n", len(items))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewTargetShowCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one Target with its Assignments and desired Skill set",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			id, err := a.ResolveTargetArg(args[0])
			if err != nil {
				return err
			}
			t, err := a.ShowTarget(id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, t)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Target %d: %s\n", t.ID, t.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\nAdapter: %s (scope %s)\n", t.Path, t.Adapter, t.Scope)
			if len(t.CompatibleAdapters) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Compatible adapters: %s\n", strings.Join(t.CompatibleAdapters, ", "))
			}
			if len(t.DirectSkills) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Direct Skill Assignments: none")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Direct Skill Assignments: %s\n", assignmentLabels(t.DirectSkills))
			}
			if len(t.Groups) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Group Assignments: none")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Group Assignments: %s\n", assignmentLabels(t.Groups))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Desired Skills: %s\n", desiredSkillLabels(t.DesiredSkills))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}
