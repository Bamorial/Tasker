package tasker

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type GitRepo struct {
	root string
}

type TaskDiffBaseline struct {
	CapturedAt string                 `json:"captured_at,omitempty"`
	Head       string                 `json:"head,omitempty"`
	Files      []TaskDiffBaselineFile `json:"files,omitempty"`
}

type TaskDiffBaselineFile struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content []byte `json:"content,omitempty"`
}

type TaskFileDiff struct {
	Path   string
	Before string
	After  string
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

func (r *GitRepo) CurrentHead() (string, error) {
	out, err := r.run("rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "ambiguous argument 'HEAD'") {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (r *GitRepo) WorkingTreeDiff() (string, error) {
	sections := make([]string, 0, 3)

	staged, err := r.run("diff", "--cached", "--no-ext-diff", "--no-color")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(staged) != "" {
		sections = append(sections, staged)
	}

	unstaged, err := r.run("diff", "--no-ext-diff", "--no-color")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(unstaged) != "" {
		sections = append(sections, unstaged)
	}

	untracked, err := r.run("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(untracked) != "" {
		sections = append(sections, "Untracked files:\n"+strings.TrimSpace(untracked))
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n")), nil
}

func ensureTaskDiffBaseline(root string, task *Task) error {
	repo, err := OpenGitRepo(root)
	if err != nil {
		return nil
	}

	var baseline TaskDiffBaseline
	ok, err := TaskContextValue(task, "git_diff_baseline", &baseline)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	captured, err := repo.captureTaskDiffBaseline()
	if err != nil {
		return err
	}

	context, err := ReadTaskContext(task)
	if err != nil {
		return err
	}
	context["git_diff_baseline"] = captured
	return WriteTaskContext(task, context)
}

func (r *GitRepo) captureTaskDiffBaseline() (TaskDiffBaseline, error) {
	head, err := r.CurrentHead()
	if err != nil {
		return TaskDiffBaseline{}, err
	}

	paths, err := r.DirtyPaths()
	if err != nil {
		return TaskDiffBaseline{}, err
	}

	files := make([]TaskDiffBaselineFile, 0, len(paths))
	for _, path := range paths {
		if !shouldIncludeTaskDiffPath(path) {
			continue
		}
		content, exists, err := readRepoFile(filepath.Join(r.root, filepath.FromSlash(path)))
		if err != nil {
			return TaskDiffBaseline{}, err
		}
		files = append(files, TaskDiffBaselineFile{
			Path:    path,
			Exists:  exists,
			Content: content,
		})
	}

	return TaskDiffBaseline{
		CapturedAt: time.Now().Format(time.RFC3339),
		Head:       head,
		Files:      files,
	}, nil
}

func (r *GitRepo) TaskDiff(task *Task) ([]TaskFileDiff, error) {
	var baseline TaskDiffBaseline
	ok, err := TaskContextValue(task, "git_diff_baseline", &baseline)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("task diff baseline missing; reopen or rerun the task to capture it")
	}

	currentPaths, err := r.DirtyPaths()
	if err != nil {
		return nil, err
	}

	pathSet := make(map[string]struct{}, len(currentPaths)+len(baseline.Files))
	for _, path := range currentPaths {
		if !shouldIncludeTaskDiffPath(path) {
			continue
		}
		pathSet[path] = struct{}{}
	}
	baselineFiles := make(map[string]TaskDiffBaselineFile, len(baseline.Files))
	for _, file := range baseline.Files {
		if !shouldIncludeTaskDiffPath(file.Path) {
			continue
		}
		baselineFiles[file.Path] = file
		pathSet[file.Path] = struct{}{}
	}

	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	diffs := make([]TaskFileDiff, 0, len(paths))
	for _, path := range paths {
		before, after, changed, err := r.diffForPath(path, baseline.Head, baselineFiles[path])
		if err != nil {
			return nil, err
		}
		if !changed {
			continue
		}
		diffs = append(diffs, TaskFileDiff{
			Path:   path,
			Before: before,
			After:  after,
		})
	}

	return diffs, nil
}

func (r *GitRepo) diffForPath(path, head string, baseline TaskDiffBaselineFile) (string, string, bool, error) {
	var beforeBytes []byte
	var beforeExists bool
	if baseline.Path != "" {
		beforeBytes = baseline.Content
		beforeExists = baseline.Exists
	} else {
		content, exists, err := r.fileContentAtHead(head, path)
		if err != nil {
			return "", "", false, err
		}
		beforeBytes = content
		beforeExists = exists
	}

	afterBytes, afterExists, err := readRepoFile(filepath.Join(r.root, filepath.FromSlash(path)))
	if err != nil {
		return "", "", false, err
	}

	if beforeExists == afterExists && bytes.Equal(beforeBytes, afterBytes) {
		return "", "", false, nil
	}

	before := renderSnapshotContent(beforeBytes, beforeExists)
	after := renderSnapshotContent(afterBytes, afterExists)
	return before, after, true, nil
}

func (r *GitRepo) fileContentAtHead(head, path string) ([]byte, bool, error) {
	if strings.TrimSpace(head) == "" {
		return nil, false, nil
	}

	spec := fmt.Sprintf("%s:%s", head, path)
	out, err := r.runBytes("show", spec)
	if err != nil {
		if strings.Contains(err.Error(), "exists on disk, but not in") || strings.Contains(err.Error(), "pathspec") || strings.Contains(err.Error(), "does not exist in") {
			return nil, false, nil
		}
		return nil, false, err
	}
	return out, true, nil
}

func (r *GitRepo) DirtyPaths() ([]string, error) {
	paths := make([]string, 0, 8)

	staged, err := r.run("diff", "--cached", "--name-only", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	paths = append(paths, parseLinePaths(staged)...)

	unstaged, err := r.run("diff", "--name-only", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	paths = append(paths, parseLinePaths(unstaged)...)

	untracked, err := r.run("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	paths = append(paths, parseLinePaths(untracked)...)

	return uniqueSortedPaths(paths), nil
}

func parseLinePaths(raw string) []string {
	lines := strings.Split(raw, "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(line))
	}
	return paths
}

func uniqueSortedPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	slices.Sort(unique)
	return unique
}

func readRepoFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func renderSnapshotContent(data []byte, exists bool) string {
	if !exists {
		return ""
	}
	if !utf8.Valid(data) {
		return "[binary content]"
	}
	return strings.ReplaceAll(string(data), "\r\n", "\n")
}

func shouldIncludeTaskDiffPath(path string) bool {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return false
	}
	return path != ".tasker" && !strings.HasPrefix(path, ".tasker/")
}

func (r *GitRepo) run(args ...string) (string, error) {
	output, err := r.runBytes(args...)
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (r *GitRepo) runBytes(args ...string) ([]byte, error) {
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
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}

	return stdout.Bytes(), nil
}
