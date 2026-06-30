package cmd

import (
	"fmt"
	"strings"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var addParentID string
var addTaskType string
var addOpenTarget string
var addNoOpen bool

var addCmd = &cobra.Command{
	Use:   "add [title]",
	Short: "Create a child task under an existing task",
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

		parentID := addParentID
		if parentID == "" {
			parentID, err = tasker.InferParentTaskID(root, ".")
			if err != nil {
				return err
			}
		}

		created, err := tasker.CreateTask(root, tasker.CreateTaskInput{
			Title:    title,
			Type:     addTaskType,
			ParentID: parentID,
		})
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created child task %s under %s\n", created.ID, parentID)
		if addNoOpen {
			return nil
		}

		openPath, err := tasker.TaskDocumentPath(created.Path, addOpenTarget)
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
	addCmd.Flags().StringVar(&addParentID, "parent", "", "Parent task ID")
	addCmd.Flags().StringVar(&addTaskType, "type", "", fmt.Sprintf("Task type (%s)", strings.Join(tasker.ValidTaskTypes(), ", ")))
	addCmd.Flags().StringVar(&addOpenTarget, "open", "task", "Document to open: task, instructions, declaration, result, meta")
	addCmd.Flags().BoolVar(&addNoOpen, "no-open", false, "Create the task without opening an editor")
}
