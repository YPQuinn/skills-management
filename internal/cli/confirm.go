package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
)

// confirmOrYes requires an explicit confirmation for a consequential
// operation. --yes skips the prompt. A non-interactive stdin cannot
// confirm and must pass --yes.
func confirmOrYes(cmd *cobra.Command, yes bool, prompt string) error {
	if yes {
		return nil
	}
	if jsonRequested {
		return app.Errorf(app.CodeInvalidArgument, "confirmation required; pass --yes")
	}
	in, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return app.Errorf(app.CodeInvalidArgument, "confirmation required; pass --yes")
	}
	info, err := in.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return app.Errorf(app.CodeInvalidArgument, "confirmation required; pass --yes")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return app.Errorf(app.CodeInvalidArgument, "cancelled")
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return app.Errorf(app.CodeInvalidArgument, "cancelled")
}
