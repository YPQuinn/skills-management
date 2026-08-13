package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// NewGroupCmd builds the noun-first `skillctl group` command group.
func NewGroupCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "group",
		Short: "Manage Groups of Skills",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		NewGroupCreateCmd(bm),
		NewGroupListCmd(bm),
		NewGroupShowCmd(bm),
		NewGroupAddSkillCmd(bm),
		NewGroupRemoveSkillCmd(bm),
	)
	return cmd
}

func NewGroupCreateCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a named Group",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			g, err := a.CreateGroup(args[0])
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, g)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created Group %q (Group %d)\n", g.Name, g.ID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewGroupListCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Groups and their membership counts",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			items, err := a.ListGroups()
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, map[string]any{"items": items, "total": len(items)})
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Groups.")
				return nil
			}
			for _, g := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%d Skill(s)\n", g.ID, g.Name, g.MemberCount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d Group(s)\n", len(items))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewGroupShowCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name-or-id>",
		Short: "Show one Group with its members and assigned Targets",
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
			g, err := a.ShowGroup(id)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, g)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Group %d: %s\n", g.ID, g.Name)
			if len(g.Members) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Members: none")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Members: %s\n", skillRefLabels(g.Members))
			}
			if len(g.Targets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Assigned Targets: none")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Assigned Targets: %s\n", targetRefLabels(g.Targets))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewGroupAddSkillCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-skill <group> <slug-or-id>...",
		Short: "Add Skills to a Group",
		Args:  minArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return groupMembership(cmd, bm, args, true)
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewGroupRemoveSkillCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-skill <group> <slug-or-id>...",
		Short: "Remove Skills from a Group",
		Args:  minArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return groupMembership(cmd, bm, args, false)
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// groupMembership runs the shared add/remove member flow: resolve the
// Group and every Skill argument, then mutate membership and report the
// refreshed Group.
func groupMembership(cmd *cobra.Command, bm *bootstrap.Manager, args []string, add bool) error {
	a, err := bm.App()
	if err != nil {
		return err
	}
	groupID, err := a.ResolveGroupArg(args[0])
	if err != nil {
		return err
	}
	skillIDs := make([]int64, 0, len(args)-1)
	for _, arg := range args[1:] {
		id, err := a.ResolveSkillArg(arg)
		if err != nil {
			return err
		}
		skillIDs = append(skillIDs, id)
	}
	var g *app.GroupView
	if add {
		g, err = a.AddGroupSkills(groupID, skillIDs)
	} else {
		g, err = a.RemoveGroupSkills(groupID, skillIDs)
	}
	if err != nil {
		return err
	}
	if jsonRequested {
		return printJSON(cmd, g)
	}
	verb := "Added"
	prep := "to"
	if !add {
		verb = "Removed"
		prep = "from"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %d Skill(s) %s Group %q (%d member(s))\n", verb, len(skillIDs), prep, g.Name, len(g.Members))
	return nil
}

// skillRefLabels renders Skill refs as "name (slug)" joined by commas.
func skillRefLabels(refs []app.SkillRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf("%q (%s)", r.Name, r.Slug))
	}
	return strings.Join(parts, ", ")
}

// targetRefLabels renders Target refs as quoted names joined by commas.
func targetRefLabels(refs []app.TargetRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, fmt.Sprintf("%q", r.Name))
	}
	return strings.Join(parts, ", ")
}
