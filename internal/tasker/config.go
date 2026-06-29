package tasker

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Editor string    `yaml:"editor"`
	Git    GitConfig `yaml:"git"`
}

type GitConfig struct {
	Enabled          bool   `yaml:"enabled"`
	BranchPerTask    bool   `yaml:"branch_per_task"`
	CommitPerSubtask bool   `yaml:"commit_per_subtask"`
	BranchPrefix     string `yaml:"branch_prefix"`
	CheckoutBranch   bool   `yaml:"checkout_branch"`
}

func LoadConfig(root string) (Config, error) {
	path := filepath.Join(root, TaskerDirName, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	applyConfigDefaults(&cfg)
	return cfg, nil
}

func applyConfigDefaults(cfg *Config) {
	if cfg.Git.BranchPrefix == "" {
		cfg.Git.BranchPrefix = "task"
	}
}
