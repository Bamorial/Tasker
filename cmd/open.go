package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/bamorial/tasker/internal/tasker"
)

var openCmd = &cobra.Command{
	Use:   "open <id>",
	Short: "Open a task in the configured editor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		task, err := tasker.GetTask(root, args[0])
		if err != nil {
			return err
		}

		if err := tasker.OpenInEditor(task.TaskFile); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Task file: %s\n", task.TaskFile)
			return nil
		}

		return nil
	},
}
