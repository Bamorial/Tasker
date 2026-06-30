package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "tasker",
	Short: "Tasker is a CLI-first universal agent workspace",
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
	rootCmd.AddCommand(metaCmd)
	rootCmd.AddCommand(treeCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(versionCmd)
}
