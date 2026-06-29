package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var instructionCmd = &cobra.Command{
	Use:   "instruction <id>",
	Short: "Open the task-specific instructions for a task",
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

		if err := tasker.OpenInEditor(task.InstructionsFile); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Task instructions: %s\n", task.InstructionsFile)
			return nil
		}

		return nil
	},
}
