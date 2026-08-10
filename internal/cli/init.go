package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"skillctl/internal/bootstrap"
)

func NewInitCmd(bm *bootstrap.Manager) *cobra.Command {
	var storePath string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the Skill Store",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := bm.Initialize(storePath); err != nil {
				return err
			}
			fmt.Println("Skill Manager initialized successfully.")
			return nil
		},
	}
	cmd.Flags().StringVar(&storePath, "store", "", "custom absolute Skill Store path")
	return cmd
}
