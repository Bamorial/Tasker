package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "tasker",
	Short: "Tasker is a CLI-first universal agent workspace",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUIFromWorkspace()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(instructionCmd)
	rootCmd.AddCommand(instructionsCmd)
	rootCmd.AddCommand(checkoutCmd)
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(doCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(metaCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(whoCmd)
}
