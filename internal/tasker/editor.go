package tasker

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func OpenInEditor(path string) error {
	editor, err := ResolveEditor(".")
	if err != nil {
		return err
	}
	if editor == "" {
		return fmt.Errorf("no editor configured; set .tasker/config.yaml `editor` or export EDITOR")
	}

	cmd := exec.Command("sh", "-c", editor+" \"$1\"", "tasker-editor", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
