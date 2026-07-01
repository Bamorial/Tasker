package tui

import (
	"strings"

	"github.com/bamorial/tasker/internal/tasker"
)

func loadKeybindings(root string) (tasker.TUIKeybindings, string) {
	cfg, err := tasker.LoadConfig(root)
	if err != nil {
		return tasker.DefaultTUIKeybindings(), "Error loading config: " + err.Error()
	}
	return cfg.TUI.Keybindings, ""
}

func keyMatches(section map[string][]string, action, key string) bool {
	for _, candidate := range section[action] {
		if candidate == key {
			return true
		}
	}
	return false
}

func keyLabel(section map[string][]string, action string) string {
	keys := section[action]
	if len(keys) == 0 {
		return action
	}
	return strings.Join(keys, "/")
}
