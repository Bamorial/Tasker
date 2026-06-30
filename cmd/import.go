package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var importParentID string
var importOpenTarget string
var importNoOpen bool
var importCheckout bool
var importBranchCheckout bool

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import tasks from a JSON document",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		importPath := ""
		if len(args) == 1 {
			importPath = args[0]
		} else {
			importPath, err = tasker.LatestImportPath(root)
			if err != nil {
				return err
			}
		}

		result, err := tasker.ImportTasks(root, importPath, tasker.ImportTaskInput{
			ParentID: importParentID,
		})
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Imported %d tasks from %s\n", len(result.Created), importPath)
		if result.Primary == nil {
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Primary task %s: %s\n", result.Primary.ID, result.Primary.Path)
		if importCheckout || importBranchCheckout {
			checkoutResult, err := tasker.CheckoutTask(root, result.Primary.ID, tasker.CheckoutTaskInput{
				NoBranch:    !importBranchCheckout,
				ForceBranch: importBranchCheckout,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Checked out primary task: %s\n", checkoutResult.Task.Meta.ID)
			if checkoutResult.Branch != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Git branch: %s\n", checkoutResult.Branch)
			}
		}

		if importNoOpen {
			return nil
		}

		openPath, err := tasker.TaskDocumentPath(result.Primary.Path, importOpenTarget)
		if err != nil {
			return err
		}

		if err := tasker.OpenInEditor(openPath); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open target: %s\n", openPath)
		}
		return nil
	},
}

var importTemplateCmd = &cobra.Command{
	Use:   "template",
	Short: "Create an editable import file in .tasker/imports and open it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		path, err := tasker.CreateImportTemplateCopy(root)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created import file: %s\n", path)
		if err := tasker.OpenInEditor(path); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open target: %s\n", path)
		}
		return nil
	},
}

func init() {
	importCmd.Flags().StringVar(&importParentID, "parent", "", "Parent task ID")
	importCmd.Flags().StringVar(&importOpenTarget, "open", "task", "Document to open: task, instructions, declaration, result, meta")
	importCmd.Flags().BoolVar(&importNoOpen, "no-open", false, "Import the task without opening an editor")
	importCmd.Flags().BoolVarP(&importCheckout, "checkout", "c", false, "Import the tasks and set the first imported root task as the current workspace without switching Git branches")
	importCmd.Flags().BoolVarP(&importBranchCheckout, "branch-checkout", "b", false, "Import the tasks, set the first imported root task as current, and create or switch to its task branch")
	importCmd.AddCommand(importTemplateCmd)
}
