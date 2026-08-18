package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
)

func printSourceDeletePreview(cmd *cobra.Command, p *app.SourceDeletePreview) {
	fmt.Fprintf(cmd.OutOrStdout(), "Source %q\n", p.Name)
	if len(p.BoundSkills) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Bound Skills: none")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Bound Skills: %s\n", skillRefsCSV(p.BoundSkills))
	fmt.Fprintln(cmd.OutOrStdout(), "These Skills will be detached, not deleted.")
}

func printSkillDeletePreview(cmd *cobra.Command, p *app.SkillDeletePreview) {
	fmt.Fprintf(cmd.OutOrStdout(), "Skill %q (%s)\n", p.Skill.Name, p.Skill.Slug)
	if len(p.Groups) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Groups: %s\n", groupRefsCSV(p.Groups))
	}
	if len(p.Targets) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Targets: %s\n", targetImpactCSV(p.Targets))
	}
	if len(p.Links) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Managed Links: %s\n", cleanupLinksCSV(p.Links))
	}
	if p.Referenced {
		fmt.Fprintln(cmd.OutOrStdout(), "Referenced: yes (requires --cleanup)")
	}
}

func printTargetDeletePreview(cmd *cobra.Command, p *app.TargetDeletePreview) {
	fmt.Fprintf(cmd.OutOrStdout(), "Target %q\n", p.Target.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Assignments: %d\n", len(p.Assignments))
	if len(p.Links) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Managed Links: %s\n", cleanupLinksCSV(p.Links))
	}
	fmt.Fprintln(cmd.OutOrStdout(), "The Target container will be kept.")
}

func printGroupDeletePreview(cmd *cobra.Command, p *app.GroupDeletePreview) {
	fmt.Fprintf(cmd.OutOrStdout(), "Group %q\n", p.Group.Name)
	if len(p.Targets) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Assigned Targets: none")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Assigned Targets: %s\n", targetRefLabels(p.Targets))
}

func printCleanupLinks(cmd *cobra.Command, links []app.CleanupLink) {
	for _, l := range links {
		if l.Result == "" {
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s @ %s: %s", l.Slug, l.TargetName, l.Result)
		if l.Warning != "" {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", l.Warning)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
}

func skillRefsCSV(refs []app.SkillRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, r.Slug)
	}
	return strings.Join(parts, ", ")
}

func groupRefsCSV(refs []app.GroupRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, r.Name)
	}
	return strings.Join(parts, ", ")
}

func targetImpactCSV(refs []app.TargetImpact) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, r.Name)
	}
	return strings.Join(parts, ", ")
}

func cleanupLinksCSV(links []app.CleanupLink) string {
	parts := make([]string, 0, len(links))
	for _, l := range links {
		parts = append(parts, fmt.Sprintf("%s@%s", l.Slug, l.TargetName))
	}
	return strings.Join(parts, ", ")
}
