package cli

import (
	"context"
	"errors"
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

// Execute builds the skillctl command tree and runs it with ctx. Failures
// are returned to the caller (which maps them onto exit codes); the human
// message is printed to stderr here, and a --json invocation additionally
// receives exactly one JSON error value on stdout so stdout stays
// machine-parseable.
func Execute(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return app.Errorf(app.CodeInternal, "cannot determine the home directory: %v", err)
	}
	root := newRootCmd(bootstrap.New(filepath.Join(home, ".skillctl", "config.toml")))
	return executeWith(ctx, root)
}

// executeWith runs one command tree and reports failures: the human message
// always goes to stderr, and --json requests also receive exactly one JSON
// error value on stdout. A partial-failure sentinel is never printed: the
// command already emitted its complete result and only needs the exit code.
func executeWith(ctx context.Context, root *cobra.Command) error {
	jsonRequested = false
	err := root.ExecuteContext(ctx)
	if err == nil {
		return nil
	}
	var pf partialFailure
	if errors.As(err, &pf) {
		return err
	}
	if jsonRequested {
		if jerr := printJSONError(root.OutOrStdout(), err); jerr != nil {
			// stdout could not take the JSON value; the human message on
			// stderr is the only remaining report.
			fmt.Fprintln(root.ErrOrStderr(), err)
			return err
		}
	}
	fmt.Fprintln(root.ErrOrStderr(), err)
	return err
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
	root.AddCommand(NewInitCmd(bm), NewStatusCmd(bm), NewUICmd(bm), NewSourceCmd(bm), NewSkillCmd(bm), NewGroupCmd(bm), NewTargetCmd(bm))
	return root
}
