package tasker

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type AgentSessionAction string

const (
	AgentSessionResume AgentSessionAction = "resume"
	AgentSessionFork   AgentSessionAction = "fork"
)

func (a AgentSessionAction) commandFor(session TaskSession) string {
	switch a {
	case AgentSessionFork:
		return strings.TrimSpace(session.ForkCommand)
	default:
		return strings.TrimSpace(session.ResumeCommand)
	}
}

func (a AgentSessionAction) label() string {
	switch a {
	case AgentSessionFork:
		return "fork"
	default:
		return "resume"
	}
}

func SessionsForAction(task *Task, action AgentSessionAction) []TaskSession {
	matches := make([]TaskSession, 0, len(task.Status.Sessions))
	for _, session := range task.Status.Sessions {
		if action.commandFor(session) == "" {
			continue
		}
		matches = append(matches, session)
	}
	return matches
}

func SelectTaskSession(task *Task, action AgentSessionAction, in *os.File, out *os.File) (*TaskSession, error) {
	sessions := SessionsForAction(task, action)
	if len(sessions) == 0 {
		return nil, fmt.Errorf("task %s has no %s-capable stored sessions", task.Meta.ID, action.label())
	}
	if len(sessions) == 1 {
		return &sessions[0], nil
	}

	if in == nil || out == nil {
		return nil, fmt.Errorf("multiple stored sessions found for task %s; rerun in a terminal to choose one", task.Meta.ID)
	}

	fmt.Fprintf(out, "Select a session to %s for task %s:\n", action.label(), task.Meta.ID)
	for i, session := range sessions {
		fmt.Fprintf(out, "%d. %s %s\n", i+1, session.Agent, session.ID)
		fmt.Fprintf(out, "   %s\n", action.commandFor(session))
	}

	reader := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "Enter choice [1-%d]: ", len(sessions))
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		choice, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || choice < 1 || choice > len(sessions) {
			fmt.Fprintln(out, "Invalid selection.")
			continue
		}

		return &sessions[choice-1], nil
	}
}

func RunTaskSessionAction(task *Task, session TaskSession, action AgentSessionAction) error {
	cmd, err := TaskSessionCommand(task, session, action)
	if err != nil {
		return err
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TaskSessionCommand(task *Task, session TaskSession, action AgentSessionAction) (*exec.Cmd, error) {
	command := action.commandFor(session)
	if command == "" {
		return nil, fmt.Errorf("task %s session %s has no %s command", task.Meta.ID, session.ID, action.label())
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = task.Path
	return cmd, nil
}
