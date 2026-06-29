package tasker

import (
	"fmt"
	"path/filepath"
	"strings"
)

type CheckoutTaskInput struct {
	Branch         string
	ExistingBranch string
	NoBranch       bool
}

type CheckoutTaskResult struct {
	Task          *Task
	Path          string
	Branch        string
	WorkspaceFile string
}

func CheckoutTask(root, id string, input CheckoutTaskInput) (*CheckoutTaskResult, error) {
	task, err := GetTask(root, id)
	if err != nil {
		return nil, err
	}

	cfg, err := LoadConfig(root)
	if err != nil {
		return nil, err
	}

	branch, err := checkoutTaskBranch(root, task, cfg, input)
	if err != nil {
		return nil, err
	}

	if err := WriteCurrentWorkspace(root, task, CurrentWorkspaceInput{
		Branch: branch,
	}); err != nil {
		return nil, err
	}

	return &CheckoutTaskResult{
		Task:          task,
		Path:          task.Path,
		Branch:        branch,
		WorkspaceFile: filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md"),
	}, nil
}

func checkoutTaskBranch(root string, task *Task, cfg Config, input CheckoutTaskInput) (string, error) {
	branchName := strings.TrimSpace(input.Branch)
	existingBranch := strings.TrimSpace(input.ExistingBranch)

	if input.NoBranch && (branchName != "" || existingBranch != "") {
		return "", fmt.Errorf("--no-branch cannot be combined with --branch or --existing-branch")
	}
	if branchName != "" && existingBranch != "" {
		return "", fmt.Errorf("--branch and --existing-branch cannot be used together")
	}
	if input.NoBranch {
		return "", nil
	}

	autoBranch := shouldAutoBranch(cfg)
	if branchName == "" && existingBranch == "" && !autoBranch {
		return "", nil
	}

	repo, err := OpenGitRepo(root)
	if err != nil {
		return "", err
	}

	switch {
	case existingBranch != "":
		if err := repo.CheckoutExistingBranch(existingBranch); err != nil {
			return "", err
		}
		return existingBranch, nil
	case branchName != "":
		if err := repo.CheckoutOrCreateBranch(branchName); err != nil {
			return "", err
		}
		return branchName, nil
	default:
		generated := TaskBranchName(task, cfg)
		if err := repo.CheckoutOrCreateBranch(generated); err != nil {
			return "", err
		}
		return generated, nil
	}
}

func shouldAutoBranch(cfg Config) bool {
	return cfg.Git.Enabled && cfg.Git.BranchPerTask && cfg.Git.CheckoutBranch
}

func TaskBranchName(task *Task, cfg Config) string {
	prefix := strings.TrimSpace(cfg.Git.BranchPrefix)
	if prefix == "" {
		prefix = "task"
	}
	return fmt.Sprintf("%s/%s-%s", prefix, task.Meta.ID, task.Meta.Slug)
}
