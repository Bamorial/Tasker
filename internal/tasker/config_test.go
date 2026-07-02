package tasker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigReturnsDefaultTUIKeybindingsWhenFileMissing(t *testing.T) {
	root := t.TempDir()

	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got := cfg.Git.BranchPrefix; got != "task" {
		t.Fatalf("expected default branch prefix %q, got %q", "task", got)
	}
	if got := cfg.TUI.Keybindings.Global["quit"]; len(got) != 2 || got[0] != "ctrl+c" || got[1] != "q" {
		t.Fatalf("expected default quit keys, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Tasks["open_current"]; len(got) != 1 || got[0] != "enter" {
		t.Fatalf("expected default task open key, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Tasks["checkout"]; len(got) != 1 || got[0] != "C" {
		t.Fatalf("expected default checkout key, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Current["show_diff"]; len(got) != 1 || got[0] != "c" {
		t.Fatalf("expected default diff view key, got %#v", got)
	}
}

func TestLoadConfigMergesCustomTUIKeybindingsWithDefaults(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, TaskerDirName), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, TaskerDirName, "config.yaml"), []byte(`tui:
  keybindings:
    global:
      refresh: ["r"]
    tasks:
      open_current: ["l"]
`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got := cfg.TUI.Keybindings.Global["refresh"]; len(got) != 1 || got[0] != "r" {
		t.Fatalf("expected overridden refresh key, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Global["quit"]; len(got) != 2 || got[1] != "q" {
		t.Fatalf("expected default quit fallback, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Tasks["open_current"]; len(got) != 1 || got[0] != "l" {
		t.Fatalf("expected overridden open_current key, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Tasks["delete_task"]; len(got) != 1 || got[0] != "d" {
		t.Fatalf("expected default delete fallback, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Tasks["checkout"]; len(got) != 1 || got[0] != "C" {
		t.Fatalf("expected default checkout fallback, got %#v", got)
	}
	if got := cfg.TUI.Keybindings.Current["show_diff"]; len(got) != 1 || got[0] != "c" {
		t.Fatalf("expected default diff key fallback, got %#v", got)
	}
}
