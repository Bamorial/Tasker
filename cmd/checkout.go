package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var checkoutBranchName string
var checkoutExistingBranch string
var checkoutNoBranch bool
var checkoutPrintPath bool

var checkoutCmd = &cobra.Command{
	Use:   "checkout <id>",
	Short: "Set the current task workspace and optionally switch Git branches",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		result, err := tasker.CheckoutTask(root, args[0], tasker.CheckoutTaskInput{
			Branch:         checkoutBranchName,
			ExistingBranch: checkoutExistingBranch,
			NoBranch:       checkoutNoBranch,
		})
		if err != nil {
			return err
		}

		if checkoutPrintPath {
			fmt.Fprintln(cmd.OutOrStdout(), result.Path)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Current task: %s %s\n", result.Task.Meta.ID, result.Task.Meta.Title)
		fmt.Fprintf(cmd.OutOrStdout(), "Task path: %s\n", result.Path)
		if result.Branch != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Git branch: %s\n", result.Branch)
		}
		if result.WorkspaceFile != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Workspace file: %s\n", result.WorkspaceFile)
		}
		return nil
	},
}

func init() {
	checkoutCmd.Flags().StringVar(&checkoutBranchName, "branch", "", "Create or reuse this branch for the task")
	checkoutCmd.Flags().StringVar(&checkoutExistingBranch, "existing-branch", "", "Link the task to an existing branch without creating a new one")
	checkoutCmd.Flags().BoolVar(&checkoutNoBranch, "no-branch", false, "Populate the current task workspace without switching branches")
	checkoutCmd.Flags().BoolVar(&checkoutPrintPath, "print-path", false, "Print the task directory path for shell wrappers")
}
