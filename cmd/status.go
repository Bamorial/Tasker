package cmd

import (
	"fmt"
	"os"

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
		opts := tasker.StatusFormatOptions{Color: statusColorEnabled(cmd.OutOrStdout())}
		if len(args) == 0 {
			rows, err = tasker.TaskStatusesStyled(root, opts)
		} else {
			rows, err = tasker.TaskStatusDetailsStyled(root, args[0], opts)
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

func statusColorEnabled(out any) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	file, ok := out.(*os.File)
	if !ok {
		return false
	}

	info, err := file.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}
