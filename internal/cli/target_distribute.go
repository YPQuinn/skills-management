package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
	"skillctl/internal/distribution"
)

// NewTargetStatusCmd shows one Target's Distribution Status, refreshing the
// observation on request.
func NewTargetStatusCmd(bm *bootstrap.Manager) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "status <name-or-id>",
		Short: "Show one Target's Distribution Status",
		Long: `Show the desired and last-observed presence of every Skill at one
Target together with observation freshness and the latest reconciliation
outcome. With --refresh a fresh coherent inspection runs first.`,
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
			var st *app.DistributionStatus
			if refresh {
				st, err = a.InspectTarget(cmd.Context(), id)
			} else {
				tv, terr := a.ShowTarget(id)
				if terr != nil {
					return terr
				}
				st = tv.Distribution
			}
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, st)
			}
			printDistributionStatus(cmd, st)
			return nil
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "run a fresh inspection first")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewTargetDistributeCmd reconciles one Target with its Assignments, with
// a dry-run plan mode.
func NewTargetDistributeCmd(bm *bootstrap.Manager) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "distribute <name-or-id>",
		Short: "Reconcile one Target with its Assignments",
		Long: `Inspect one Target and create missing desired links before removing
no-longer-desired Managed Links. Creation is atomic and never overwrites an
existing entry; removal only ever unlinks a symlink that still matches its
ownership record. --dry-run prints the predicted plan without mutating.`,
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
			res, err := a.DistributeTarget(cmd.Context(), id, dryRun)
			if err != nil {
				return err
			}
			if jsonRequested {
				if err := printJSON(cmd, res); err != nil {
					return err
				}
			} else {
				printDistributionResult(cmd, res)
			}
			if res.Outcome != distribution.ResultSucceeded {
				return exitPartial("Target %d distribution: %s", id, res.Outcome)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the predicted plan without mutating")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// NewTargetAdoptCmd explicitly adopts one eligible existing symlink as a
// Managed Link.
func NewTargetAdoptCmd(bm *bootstrap.Manager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt <name-or-id> <skill-slug-or-id>",
		Short: "Adopt an existing symlink as a Managed Link",
		Long: `Adopt one existing symlink at the Target whose fresh physical
resolution points exactly at the currently desired Store Skill. The link is
never rewritten; its existing raw target is recorded as ownership.`,
		Args: exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := bm.App()
			if err != nil {
				return err
			}
			targetID, err := a.ResolveTargetArg(args[0])
			if err != nil {
				return err
			}
			skillID, err := a.ResolveSkillArg(args[1])
			if err != nil {
				return err
			}
			res, err := a.AdoptTargetLink(cmd.Context(), targetID, skillID)
			if err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Adopted %q at Target %d: %s -> %s\n",
				res.Slug, res.TargetID, res.LinkPath, res.RawTarget)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

// printDistributionStatus renders the human status view.
func printDistributionStatus(cmd *cobra.Command, st *app.DistributionStatus) {
	fmt.Fprintf(cmd.OutOrStdout(), "Target %d: %s\n", st.TargetID, st.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\n", st.Path)
	state := st.State
	if state == "" {
		state = "stored"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "State: %s", state)
	if st.InspectedAt != nil {
		fmt.Fprintf(cmd.OutOrStdout(), " (inspected %s)", st.InspectedAt.UTC().Format(timeRFC3339))
	}
	if st.Stale {
		fmt.Fprint(cmd.OutOrStdout(), " [stale]")
	}
	fmt.Fprintln(cmd.OutOrStdout())
	if st.StateError != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", st.StateError)
	}
	if len(st.Items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No Skills to distribute.")
	}
	for _, it := range st.Items {
		line := fmt.Sprintf("  %s: %s + %s", it.Slug, it.Desired, it.Observed)
		if it.Adoptable {
			line += " (adoptable)"
		}
		if it.LastResult != "" {
			line += fmt.Sprintf(" [last: %s]", it.LastResult)
		}
		if it.Stale {
			line += " [stale]"
		}
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	if st.LastResult != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Latest outcome: %s", st.LastResult)
		if st.LastCompletedAt != nil {
			fmt.Fprintf(cmd.OutOrStdout(), " (%s)", st.LastCompletedAt.UTC().Format(timeRFC3339))
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
}

// printDistributionResult renders the human reconciliation result.
func printDistributionResult(cmd *cobra.Command, res *app.DistributionResult) {
	mode := "distribution"
	if res.DryRun {
		mode = "plan"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Target %d %s: %s\n", res.TargetID, mode, res.Outcome)
	if res.Error != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Error: %s\n", res.Error)
	}
	for _, item := range res.Items {
		line := fmt.Sprintf("  %s %q: %s", item.Result, item.Slug, item.Action)
		if item.Error != "" {
			line += fmt.Sprintf(" (%s)", item.Error)
		}
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	if !res.DryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Summary: %d total, %d created, %d removed, %d adopted, %d blocked, %d failed\n",
			res.Summary.Total, res.Summary.Created, res.Summary.Removed, res.Summary.Adopted,
			res.Summary.BlockedConflict+res.Summary.BlockedBroken+res.Summary.OwnershipLost, res.Summary.Failed)
	}
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"
