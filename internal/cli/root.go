package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
)

// version and gitCommit are injected at release build time through -ldflags
// (`-X skillctl/internal/cli.version=v0.1.0 -X skillctl/internal/cli.gitCommit=<sha>`);
// development builds report the defaults. No build timestamp is recorded.
var (
	version   = "dev"
	gitCommit = "unknown"
)

// Execute builds the skillctl command tree and runs it with ctx.
func Execute(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return app.Errorf(app.CodeInternal, "cannot determine the home directory: %v", err)
	}
	return newRootCmd(bootstrap.New(filepath.Join(home, ".skillctl", "config.toml"))).ExecuteContext(ctx)
}

func newRootCmd(bm *bootstrap.Manager) *cobra.Command {
	var showVersion bool
	root := &cobra.Command{
		Use:           "skillctl",
		Short:         "Skill Manager CLI",
		Args:          noArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Fprintf(cmd.OutOrStdout(), "skillctl %s (%s)\n", version, gitCommit)
				return nil
			}
			if len(args) > 0 {
				return app.Errorf(app.CodeInvalidArgument, "unknown command %q for %q", args[0], cmd.Name())
			}
			return cmd.Help()
		},
	}
	root.Flags().BoolVar(&showVersion, "version", false, "print the version and exit")
	// Unknown flags and unknown commands are argument-validation errors.
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return app.Errorf(app.CodeInvalidArgument, "%v", err)
	})
	root.AddCommand(NewInitCmd(bm), NewStatusCmd(bm), NewUICmd(bm))
	return root
}
