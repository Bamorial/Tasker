package tasker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func OpenInEditor(path string) error {
	cmd, err := EditorCommand(".", path)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func EditorCommand(start, path string) (*exec.Cmd, error) {
	editor, err := ResolveEditor(start)
	if err != nil {
		return nil, err
	}
	if editor == "" {
		return nil, fmt.Errorf("no editor configured; set .tasker/config.yaml `editor` or export EDITOR")
	}

	return exec.Command("sh", "-c", editor+" \"$1\"", "tasker-editor", path), nil
}

func ResolveEditor(start string) (string, error) {
	root, err := FindWorkspaceRoot(start)
	if err == nil {
		cfg, cfgErr := LoadConfig(root)
		if cfgErr != nil {
			return "", cfgErr
		}
		if editor := strings.TrimSpace(cfg.Editor); editor != "" {
			return editor, nil
		}
	}

	return strings.TrimSpace(os.Getenv("EDITOR")), nil
}
