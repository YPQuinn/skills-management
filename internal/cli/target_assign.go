package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

func NewTargetAssignCmd(bm *bootstrap.Manager) *cobra.Command {
	var skill, group string
	cmd := &cobra.Command{
		Use:   "assign <name-or-id>",
		Short: "Assign a Skill or Group to a Target",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return targetAssignment(cmd, bm, args[0], skill, group, true)
		},
	}
	cmd.Flags().StringVar(&skill, "skill", "", "Skill slug or id")
	cmd.Flags().StringVar(&group, "group", "", "Group name or id")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func NewTargetUnassignCmd(bm *bootstrap.Manager) *cobra.Command {
	var skill, group string
	cmd := &cobra.Command{
		Use:   "unassign <name-or-id>",
		Short: "Remove a Skill or Group Assignment from a Target",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return targetAssignment(cmd, bm, args[0], skill, group, false)
		},
	}
	cmd.Flags().StringVar(&skill, "skill", "", "Skill slug or id")
	cmd.Flags().StringVar(&group, "group", "", "Group name or id")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// targetAssignment runs the shared assign/unassign flow: exactly one of
// --skill or --group must be given, the Target and subject resolve, and
// the mutation only changes desired state.
func targetAssignment(cmd *cobra.Command, bm *bootstrap.Manager, targetArg, skill, group string, assign bool) error {
	if (skill == "") == (group == "") {
		return app.Errorf(app.CodeInvalidArgument, "exactly one of --skill or --group is required")
	}
	a, err := bm.App()
	if err != nil {
		return err
	}
	targetID, err := a.ResolveTargetArg(targetArg)
	if err != nil {
		return err
	}
	if assign {
		in := app.AssignInput{Kind: "skill"}
		var subjectLabel string
		if skill != "" {
			in.SkillID, err = a.ResolveSkillArg(skill)
			subjectLabel = skill
		} else {
			in.Kind = "group"
			in.GroupID, err = a.ResolveGroupArg(group)
			subjectLabel = group
		}
		if err != nil {
			return err
		}
		as, err := a.AssignTarget(targetID, in)
		if err != nil {
			return err
		}
		if jsonRequested {
			return printJSON(cmd, as)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Assigned %s %q to Target %d\n", as.Kind, subjectLabel, targetID)
		return nil
	}
	kind := "skill"
	subject := ""
	var subjectID int64
	if skill != "" {
		subjectID, err = a.ResolveSkillArg(skill)
		subject = skill
	} else {
		kind = "group"
		subjectID, err = a.ResolveGroupArg(group)
		subject = group
	}
	if err != nil {
		return err
	}
	view, err := a.UnassignTarget(targetID, kind, subjectID)
	if err != nil {
		return err
	}
	if jsonRequested {
		return printJSON(cmd, view)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s Assignment %q from Target %d\n", kind, subject, targetID)
	return nil
}

// assignmentLabels renders Assignment views as quoted subject names.
func assignmentLabels(items []app.AssignmentView) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		if it.Skill != nil {
			parts = append(parts, fmt.Sprintf("%q (%s)", it.Skill.Name, it.Skill.Slug))
		} else if it.Group != nil {
			parts = append(parts, fmt.Sprintf("%q (group)", it.Group.Name))
		}
	}
	return strings.Join(parts, ", ")
}

// desiredSkillLabels renders the desired set with its reasons.
func desiredSkillLabels(items []app.DesiredSkill) string {
	if len(items) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(items))
	for _, d := range items {
		reasons := make([]string, 0, len(d.Reasons))
		for _, r := range d.Reasons {
			if r.Kind == "skill" {
				reasons = append(reasons, "direct")
			} else {
				reasons = append(reasons, "group "+r.GroupName)
			}
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", d.Slug, strings.Join(reasons, ", ")))
	}
	return strings.Join(parts, "; ")
}
