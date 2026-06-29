package tasker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const TaskerDirName = ".tasker"

func WorkingDir() (string, error) {
	return os.Getwd()
}

func FindWorkspaceRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(current, TaskerDirName)
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("tasker workspace not initialized; run `tasker init` first")
		}
		current = parent
	}
}

func InitializeWorkspace(root string) error {
	dirs := []string{
		filepath.Join(root, TaskerDirName),
		filepath.Join(root, TaskerDirName, "tasks"),
		filepath.Join(root, TaskerDirName, "refs"),
		filepath.Join(root, TaskerDirName, "memory"),
		filepath.Join(root, TaskerDirName, "current"),
		filepath.Join(root, TaskerDirName, "sessions"),
		filepath.Join(root, TaskerDirName, "logs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	files := map[string]string{
		filepath.Join(root, "AGENTS.md"):                               agentsTemplate(),
		filepath.Join(root, TaskerDirName, "START.md"):                 startTemplate(),
		filepath.Join(root, TaskerDirName, "instructions.md"):          instructionsTemplate(),
		filepath.Join(root, TaskerDirName, "agent.md"):                 agentTemplate(),
		filepath.Join(root, TaskerDirName, "config.yaml"):              configTemplate(),
		filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md"):  workspaceTemplate(),
		filepath.Join(root, TaskerDirName, "current", "FILES.md"):      filesTemplate(),
		filepath.Join(root, TaskerDirName, "current", "CONTEXT.json"):  "{}\n",
	}

	for path, content := range files {
		if err := writeFileIfMissing(path, content); err != nil {
			return err
		}
	}

	return nil
}

func writeFileIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
