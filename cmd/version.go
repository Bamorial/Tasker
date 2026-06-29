package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/buildinfo"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show build version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "tasker %s\n", buildinfo.Version)
		fmt.Fprintf(cmd.OutOrStdout(), "commit: %s\n", buildinfo.Commit)
		fmt.Fprintf(cmd.OutOrStdout(), "built: %s\n", buildinfo.Date)
	},
}
