package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/bamorial/tasker/internal/tasker"
)

var instructionsCmd = &cobra.Command{
	Use:   "instructions",
	Short: "Open the project-wide Tasker instructions",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		path := filepath.Join(root, tasker.TaskerDirName, "instructions.md")
		if err := tasker.OpenInEditor(path); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Instructions: %s\n", path)
			return nil
		}

		return nil
	},
}
