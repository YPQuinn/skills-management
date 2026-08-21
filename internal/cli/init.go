package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
	"skillctl/internal/target"
)

func NewInitCmd(bm *bootstrap.Manager) *cobra.Command {
	var storePath string
	var recoverStore bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Skill Store",
		Long: `Initialize Skill Manager. Writes ~/.skillctl/config.toml, creates
state.db, and creates the Skill Store at ~/.skillctl/store unless --store
names another absolute directory.

If config.toml exists but state.db is missing, ordinary commands report
state_missing and refuse to invent empty state. --recover-store rebuilds
state from valid top-level Store directories as unbound Skills. It does
not rewrite Store content and cannot restore Sources, Groups, Assignments,
Targets, or Managed Link ownership. --store cannot be combined with
--recover-store.`,
		Example: `  skillctl init
  skillctl init --store /abs/path/to/store
  skillctl init --recover-store`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if recoverStore {
				if storePath != "" {
					return app.Errorf(app.CodeInvalidArgument, "--store cannot be combined with --recover-store")
				}
				result, err := bm.RecoverStore()
				if err != nil {
					return err
				}
				if jsonRequested {
					return printJSON(cmd, result)
				}
				printStoreRecovery(cmd, result)
				return nil
			}
			if err := bm.Initialize(storePath); err != nil {
				return err
			}
			if jsonRequested {
				return printJSON(cmd, map[string]any{"state": "ready"})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Skill Manager initialized successfully.")
			// Detection is advisory and read-only (decision 04): show which
			// built-in adapters appear installed so the operator can pick
			// suggested Targets with `skillctl target add`.
			var detected []string
			for _, d := range target.DetectAll(target.DefaultDetectionOptions()) {
				if d.Status == target.StatusDetected {
					detected = append(detected, d.Adapter)
				}
			}
			if len(detected) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Detected Target Adapters: %s\n", strings.Join(detected, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&storePath, "store", "", "custom absolute Skill Store path")
	cmd.Flags().BoolVar(&recoverStore, "recover-store", false, "rebuild missing state.db from the configured Skill Store")
	cmd.Flags().BoolVar(&jsonRequested, "json", false, "print one JSON value on stdout")
	return cmd
}

func printStoreRecovery(cmd *cobra.Command, result *app.StoreRecoveryResult) {
	fmt.Fprintln(cmd.OutOrStdout(), "Skill Manager recovered from the Skill Store.")
	if len(result.Recovered) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Recovered 0 unbound Skill(s).")
	} else {
		slugs := make([]string, 0, len(result.Recovered))
		for _, s := range result.Recovered {
			slugs = append(slugs, s.Slug)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Recovered %d unbound Skill(s): %s\n", len(result.Recovered), strings.Join(slugs, ", "))
	}
	if len(result.PreservedInternal) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Preserved unprovable internal trees: %s\n", strings.Join(result.PreservedInternal, ", "))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Unrecoverable: %s\n", strings.Join(result.Unrecoverable, ", "))
}
