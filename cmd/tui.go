package cmd

import (
	"github.com/bamorial/tasker/internal/tasker"
	"github.com/bamorial/tasker/internal/tui"
	"github.com/spf13/cobra"
)

var runTUI = tui.Run

func runTUIFromWorkspace() error {
	root, err := tasker.FindWorkspaceRoot(".")
	if err != nil {
		return err
	}
	return runTUI(root)
}

var tuiCmd = &cobra.Command{
	Use:    "tui",
	Short:  "Open the Tasker terminal UI",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUIFromWorkspace()
	},
}
