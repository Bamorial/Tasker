package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var deleteRecursive bool

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		if err := tasker.DeleteTask(root, args[0], deleteRecursive); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleted task %s\n", args[0])
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&deleteRecursive, "recursive", false, "Delete the task and all child tasks")
}
