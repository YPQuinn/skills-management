package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
)

// jsonRequested records whether the command being executed requested --json
// output. It is bound by every command that supports the flag and reset at
// the start of each Execute, so stale state never leaks between runs.
var jsonRequested bool

// partialFailure marks a command that already printed its complete result
// on stdout (human or --json) and only needs the exit code 1: a
// synchronization batch with skipped, blocked, or failed items, or a
// single-skill action whose use case was blocked. It is never printed,
// because a second report would corrupt the single-JSON-value stdout
// contract (decision 09).
type partialFailure struct{ msg string }

func (p partialFailure) Error() string { return p.msg }

// exitPartial returns a partial-failure sentinel for one use case that was
// blocked, partial, or failed but already reported its complete result.
func exitPartial(format string, args ...any) partialFailure {
	return partialFailure{msg: fmt.Sprintf(format, args...)}
}

// jsonErrorResult is the CLI --json failure shape: the same error envelope
// the REST boundary uses, without the HTTP layer.
type jsonErrorResult struct {
	Error jsonErrorBody `json:"error"`
}

type jsonErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// printJSONError writes exactly one JSON error value to w. An untyped error
// maps to the stable internal code; details always encodes as an empty
// object so the shape never varies.
func printJSONError(w io.Writer, err error) error {
	body := jsonErrorBody{Code: app.CodeInternal, Message: err.Error(), Details: map[string]any{}}
	var ae *app.Error
	if errors.As(err, &ae) {
		body.Code = ae.Code
		body.Message = ae.Message
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonErrorResult{Error: body})
}

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

// exactArgs requires exactly n positional arguments as argument-validation
// errors.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return app.Errorf(app.CodeInvalidArgument, "accepts %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}

// minArgs requires at least n positional arguments as argument-validation
// errors.
func minArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return app.Errorf(app.CodeInvalidArgument, "accepts at least %d arg(s), received %d", n, len(args))
		}
		return nil
	}
}
