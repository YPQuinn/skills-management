package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/bootstrap"
)

func NewStatusCmd(bm *bootstrap.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the Skill Manager state",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := bm.Detect()
			if err != nil {
				// invalid configuration is a validation failure: exit 2
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", st)
			return nil
		},
	}
}
