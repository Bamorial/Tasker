package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var whoCmd = &cobra.Command{
	Use:   "who",
	Short: "Print the Tasker identity string",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "tasker me")
		return nil
	},
}
