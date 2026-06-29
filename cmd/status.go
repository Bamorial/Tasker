package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [id]",
	Short: "Show task statuses",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		var rows []string
		if len(args) == 0 {
			rows, err = tasker.TaskStatuses(root)
		} else {
			rows, err = tasker.TaskStatusDetails(root, args[0])
		}
		if err != nil {
			return err
		}

		for _, row := range rows {
			fmt.Fprintln(cmd.OutOrStdout(), row)
		}
		return nil
	},
}
