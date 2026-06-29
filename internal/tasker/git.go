package tasker

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type GitRepo struct {
	root string
}

func OpenGitRepo(root string) (*GitRepo, error) {
	repo := &GitRepo{root: root}
	if _, err := repo.run("rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, fmt.Errorf("git repository not available: %w", err)
	}
	return repo, nil
}

func (r *GitRepo) CheckoutOrCreateBranch(branch string) error {
	exists, err := r.BranchExists(branch)
	if err != nil {
		return err
	}
	if exists {
		return r.CheckoutExistingBranch(branch)
	}
	_, err = r.run("checkout", "-b", branch)
	return err
}

func (r *GitRepo) CheckoutExistingBranch(branch string) error {
	exists, err := r.BranchExists(branch)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("git branch %q does not exist", branch)
	}
	_, err = r.run("checkout", branch)
	return err
}

func (r *GitRepo) BranchExists(branch string) (bool, error) {
	out, err := r.run("branch", "--list", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (r *GitRepo) CurrentBranch() (string, error) {
	out, err := r.run("branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *GitRepo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	return stdout.String(), nil
}
