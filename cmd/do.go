package cmd

import (
	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var doCmd = &cobra.Command{
	Use:   "do <id>",
	Short: "Run a task in a new headless Codex session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		return tasker.DoTask(root, args[0], cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}
