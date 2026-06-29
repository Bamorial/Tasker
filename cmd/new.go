package cmd

import (
	"fmt"
	"strings"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var newTaskType string
var newOpenTarget string
var newNoOpen bool

var newCmd = &cobra.Command{
	Use:   "new [title]",
	Short: "Create a top-level task",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		title := "Untitled task"
		if len(args) == 1 {
			title = args[0]
		}

		created, err := tasker.CreateTask(root, tasker.CreateTaskInput{
			Title: title,
			Type:  newTaskType,
		})
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created task %s: %s\n", created.ID, created.Path)
		if newNoOpen {
			return nil
		}

		openPath, err := tasker.TaskDocumentPath(created.Path, newOpenTarget)
		if err != nil {
			return err
		}

		if err := tasker.OpenInEditor(openPath); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Open target: %s\n", openPath)
		}
		return nil
	},
}

func init() {
	newCmd.Flags().StringVar(&newTaskType, "type", "feature", fmt.Sprintf("Task type (%s)", strings.Join(tasker.ValidTaskTypes(), ", ")))
	newCmd.Flags().StringVar(&newOpenTarget, "open", "task", "Document to open: task, instructions, declaration, result, meta")
	newCmd.Flags().BoolVar(&newNoOpen, "no-open", false, "Create the task without opening an editor")
}
