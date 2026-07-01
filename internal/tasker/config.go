package tasker

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Editor string    `yaml:"editor"`
	Git    GitConfig `yaml:"git"`
	TUI    TUIConfig `yaml:"tui"`
}

type GitConfig struct {
	Enabled          bool   `yaml:"enabled"`
	BranchPerTask    bool   `yaml:"branch_per_task"`
	CommitPerSubtask bool   `yaml:"commit_per_subtask"`
	BranchPrefix     string `yaml:"branch_prefix"`
	CheckoutBranch   bool   `yaml:"checkout_branch"`
}

type TUIConfig struct {
	Keybindings TUIKeybindings `yaml:"keybindings"`
}

type TUIKeybindings struct {
	Global   map[string][]string `yaml:"global"`
	Tasks    map[string][]string `yaml:"tasks"`
	Current  map[string][]string `yaml:"current"`
	Workers  map[string][]string `yaml:"workers"`
	Viewport map[string][]string `yaml:"viewport"`
	Filter   map[string][]string `yaml:"filter"`
	Form     map[string][]string `yaml:"form"`
	Session  map[string][]string `yaml:"session"`
	Confirm  map[string][]string `yaml:"confirm"`
}

func LoadConfig(root string) (Config, error) {
	path := filepath.Join(root, TaskerDirName, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := defaultConfig()
			return cfg, nil
		}
		return Config{}, err
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	applyConfigDefaults(&cfg)
	return cfg, nil
}

func DefaultTUIKeybindings() TUIKeybindings {
	return TUIKeybindings{
		Global: map[string][]string{
			"quit":                {"ctrl+c", "q"},
			"focus_current":       {"0"},
			"focus_tasks":         {"1"},
			"focus_workers":       {"2"},
			"toggle_help":         {"?"},
			"refresh":             {"R"},
			"filter":              {"/"},
			"cycle_status_filter": {"S"},
			"cycle_type_filter":   {"T"},
		},
		Tasks: map[string][]string{
			"move_up":                {"up", "k"},
			"move_down":              {"down", "j"},
			"page_up":                {"pgup"},
			"page_down":              {"pgdown"},
			"new_task":               {"n"},
			"add_child":              {"a"},
			"edit_meta":              {"m"},
			"checkout":               {"c"},
			"import_tasks":           {"u"},
			"create_import_template": {"I"},
			"delete_task":            {"d"},
			"open_doc":               {"e"},
			"run_do":                 {"x"},
			"resume":                 {"*"},
			"fork_session":           {"f"},
			"open_output":            {"o"},
			"open_current":           {"enter"},
		},
		Current: map[string][]string{
			"show_task":    {"t"},
			"show_result":  {"r"},
			"show_status":  {"s"},
			"show_agent":   {"w"},
			"open_output":  {"o"},
			"edit_doc":     {"e"},
			"run_do":       {"x"},
			"resume":       {"*"},
			"fork_session": {"F"},
		},
		Workers: map[string][]string{
			"move_up":     {"up", "k"},
			"move_down":   {"down", "j"},
			"page_up":     {"pgup"},
			"page_down":   {"pgdown"},
			"open_output": {"enter"},
			"stop_task":   {"d"},
		},
		Viewport: map[string][]string{
			"line_up":   {"up", "k", "ctrl+p"},
			"line_down": {"down", "j", "ctrl+n"},
			"page_up":   {"pgup", "b"},
			"page_down": {"pgdown", "space"},
			"top":       {"g", "home"},
			"bottom":    {"G", "end"},
		},
		Filter: map[string][]string{
			"cancel": {"esc"},
			"apply":  {"enter"},
		},
		Form: map[string][]string{
			"cancel":         {"esc"},
			"next_field":     {"tab", "down"},
			"prev_field":     {"shift+tab", "up"},
			"prev_option":    {"left"},
			"next_option":    {"right"},
			"submit":         {"ctrl+s"},
			"submit_or_next": {"enter"},
		},
		Session: map[string][]string{
			"cancel":    {"esc"},
			"move_up":   {"up", "k"},
			"move_down": {"down", "j"},
			"select":    {"enter"},
		},
		Confirm: map[string][]string{
			"cancel":           {"esc"},
			"toggle_choice":    {"left", "h", "right", "l"},
			"toggle_recursive": {"r"},
			"accept":           {"enter"},
		},
	}
}

func defaultConfig() Config {
	cfg := Config{}
	applyConfigDefaults(&cfg)
	return cfg
}

func applyConfigDefaults(cfg *Config) {
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "task"
	}
	cfg.TUI.Keybindings = mergeTUIKeybindings(cfg.TUI.Keybindings, DefaultTUIKeybindings())
}

func mergeTUIKeybindings(current, defaults TUIKeybindings) TUIKeybindings {
	current.Global = mergeKeybindingSection(current.Global, defaults.Global)
	current.Tasks = mergeKeybindingSection(current.Tasks, defaults.Tasks)
	current.Current = mergeKeybindingSection(current.Current, defaults.Current)
	current.Workers = mergeKeybindingSection(current.Workers, defaults.Workers)
	current.Viewport = mergeKeybindingSection(current.Viewport, defaults.Viewport)
	current.Filter = mergeKeybindingSection(current.Filter, defaults.Filter)
	current.Form = mergeKeybindingSection(current.Form, defaults.Form)
	current.Session = mergeKeybindingSection(current.Session, defaults.Session)
	current.Confirm = mergeKeybindingSection(current.Confirm, defaults.Confirm)
	return current
}

func mergeKeybindingSection(current, defaults map[string][]string) map[string][]string {
	if current == nil {
		current = make(map[string][]string, len(defaults))
	}
	for action, keys := range defaults {
		if len(current[action]) == 0 {
			current[action] = append([]string(nil), keys...)
		}
	}
	return current
}
