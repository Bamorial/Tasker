package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/bamorial/tasker/internal/tasker"
)

var treeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Show the task hierarchy",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		tree, err := tasker.TaskTree(root)
		if err != nil {
			return err
		}

		for _, line := range tree {
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
		return nil
	},
}
