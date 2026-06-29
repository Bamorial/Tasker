package cmd

import (
	"fmt"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var metaTitle string
var metaTaskType string
var metaOpen bool

var metaCmd = &cobra.Command{
	Use:   "meta <id>",
	Short: "Open or update a task's metadata",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		taskID := args[0]
		hasUpdates := metaTitle != "" || metaTaskType != ""

		task, err := tasker.GetTask(root, taskID)
		if err != nil {
			return err
		}

		if hasUpdates {
			task, err = tasker.UpdateTaskMeta(root, taskID, tasker.UpdateTaskMetaInput{
				Title: metaTitle,
				Type:  metaTaskType,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated task %s metadata\n", task.Meta.ID)
		}

		if !hasUpdates || metaOpen {
			if err := tasker.OpenInEditor(task.MetaFile); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Metadata file: %s\n", task.MetaFile)
			}
		}

		return nil
	},
}

func init() {
	metaCmd.Flags().StringVar(&metaTitle, "title", "", "Update the task title")
	metaCmd.Flags().StringVar(&metaTaskType, "type", "", "Update the task type")
	metaCmd.Flags().BoolVar(&metaOpen, "open", false, "Open meta.json after applying any updates")
}
