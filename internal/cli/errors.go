package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
)

// ExitCode maps an Execute error onto the stable exit codes: 2 for argument
// or configuration validation, 1 for blocked or failed use cases.
func ExitCode(err error) int {
	var ae *app.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case app.CodeInvalidArgument, app.CodeInvalidConfig:
			return 2
		}
	}
	return 1
}

// noArgs rejects positional arguments as argument-validation errors.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return app.Errorf(app.CodeInvalidArgument, "unknown command %q for %q", args[0], cmd.Root().Name())
	}
	return nil
}
