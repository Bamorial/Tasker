package cmd

import (
	"fmt"
	"os"

	"github.com/bamorial/tasker/internal/tasker"
	"github.com/spf13/cobra"
)

var resumeFork bool

var resumeCmd = &cobra.Command{
	Use:   "resume <id>",
	Short: "Resume or fork a stored agent session for a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := tasker.FindWorkspaceRoot(".")
		if err != nil {
			return err
		}

		task, err := tasker.GetTask(root, args[0])
		if err != nil {
			return err
		}

		action := tasker.AgentSessionResume
		if resumeFork {
			action = tasker.AgentSessionFork
		}

		session, err := tasker.SelectTaskSession(task, action, os.Stdin, os.Stdout)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s session: %s %s\n", actionLabel(action), session.Agent, session.ID)
		return tasker.RunTaskSessionAction(task, *session, action)
	},
}

func actionLabel(action tasker.AgentSessionAction) string {
	if action == tasker.AgentSessionFork {
		return "Forking"
	}
	return "Resuming"
}

func init() {
	resumeCmd.Flags().BoolVarP(&resumeFork, "fork", "f", false, "Fork the stored session instead of resuming it")
}
