package tasker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type TaskExecutionState struct {
	PID       int    `json:"pid"`
	PGID      int    `json:"pgid,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
}

func TaskExecutionStatePath(task *Task) string {
	return filepath.Join(task.Path, "sessions", "execution.json")
}

func WriteTaskExecutionState(task *Task, state TaskExecutionState) error {
	return writeJSON(TaskExecutionStatePath(task), state)
}

func ReadTaskExecutionState(task *Task) (TaskExecutionState, error) {
	var state TaskExecutionState
	if err := readJSON(TaskExecutionStatePath(task), &state); err != nil {
		return TaskExecutionState{}, err
	}
	return state, nil
}

func ClearTaskExecutionState(task *Task) error {
	err := os.Remove(TaskExecutionStatePath(task))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func CurrentTaskExecutionState(startedAt time.Time) TaskExecutionState {
	state := TaskExecutionState{
		PID:       os.Getpid(),
		StartedAt: startedAt.Format(time.RFC3339),
	}

	pgid, err := syscall.Getpgid(state.PID)
	if err == nil && pgid == state.PID {
		state.PGID = pgid
	}

	return state
}

func StopTaskExecution(root, id string) error {
	task, err := GetTask(root, id)
	if err != nil {
		return err
	}

	state, err := ReadTaskExecutionState(task)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if stopErr := signalTaskExecution(state); stopErr != nil {
			return stopErr
		}
	}

	if clearErr := ClearTaskExecutionState(task); clearErr != nil {
		return clearErr
	}
	return UpdateTaskStatus(task, "CANCELLED", "codex", time.Now())
}

func signalTaskExecution(state TaskExecutionState) error {
	if state.PGID > 0 {
		if err := syscall.Kill(-state.PGID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("stop task execution group %d: %w", state.PGID, err)
		}
		return nil
	}
	if state.PID <= 0 {
		return fmt.Errorf("task execution state is missing a pid")
	}
	if err := syscall.Kill(state.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("stop task execution pid %d: %w", state.PID, err)
	}
	return nil
}
