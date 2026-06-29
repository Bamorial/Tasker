package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/bamorial/tasker/internal/tasker"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Tasker in the current repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := tasker.WorkingDir()
		if err != nil {
			return err
		}

		if err := tasker.InitializeWorkspace(cwd); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Initialized Tasker in %s\n", cwd)
		return nil
	},
}
