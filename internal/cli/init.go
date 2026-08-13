package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"skillctl/internal/bootstrap"
	"skillctl/internal/target"
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
				fmt.Printf("Detected Target Adapters: %s\n", strings.Join(detected, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&storePath, "store", "", "custom absolute Skill Store path")
	return cmd
}
