package tasker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CurrentWorkspaceInput struct {
	Branch string
}

func WriteCurrentWorkspace(root string, task *Task, input CurrentWorkspaceInput) error {
	if err := ensureTaskDiffBaseline(root, task); err != nil {
		return err
	}

	chain, err := taskParentChain(root, task)
	if err != nil {
		return err
	}

	workspacePath := filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md")
	filesPath := filepath.Join(root, TaskerDirName, "current", "FILES.md")
	contextPath := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")

	if err := os.WriteFile(workspacePath, []byte(renderWorkspaceMarkdown(root, chain, task, input)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filesPath, []byte(renderCurrentFilesMarkdown(root, chain, task)), 0o644); err != nil {
		return err
	}
	if err := writeJSON(contextPath, buildCurrentContext(root, chain, task, input)); err != nil {
		return err
	}

	return nil
}

func ClearCurrentWorkspace(root string) error {
	workspacePath := filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md")
	filesPath := filepath.Join(root, TaskerDirName, "current", "FILES.md")
	contextPath := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")

	if err := os.WriteFile(workspacePath, []byte(workspaceTemplate()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filesPath, []byte(filesTemplate()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(contextPath, []byte("{}\n"), 0o644); err != nil {
		return err
	}

	return nil
}

func taskParentChain(root string, task *Task) ([]Task, error) {
	if task.Meta.ParentID == "" {
		return nil, nil
	}

	chain := make([]Task, 0)
	parentID := task.Meta.ParentID
	for parentID != "" {
		parent, err := GetTask(root, parentID)
		if err != nil {
			return nil, err
		}
		chain = append([]Task{*parent}, chain...)
		parentID = parent.Meta.ParentID
	}

	return chain, nil
}

func renderWorkspaceMarkdown(root string, chain []Task, task *Task, input CurrentWorkspaceInput) string {
	var b strings.Builder

	b.WriteString("# Current Workspace\n\n")
	b.WriteString("Current task:\n")
	b.WriteString(fmt.Sprintf("- %s %s\n", task.Meta.ID, task.Meta.Title))
	b.WriteString(fmt.Sprintf("- Type: %s\n", task.Meta.Type))
	b.WriteString(fmt.Sprintf("- Path: %s\n", relativePath(root, task.Path)))
	if input.Branch != "" {
		b.WriteString(fmt.Sprintf("- Git branch: %s\n", input.Branch))
	}
	b.WriteString("\n")

	b.WriteString("Parent tasks:\n")
	if len(chain) == 0 {
		b.WriteString("- none\n\n")
	} else {
		for _, parent := range chain {
			b.WriteString(fmt.Sprintf("- %s %s\n", parent.Meta.ID, parent.Meta.Title))
		}
		b.WriteString("\n")
	}

	b.WriteString("Rules:\n")
	b.WriteString("- Read .tasker/instructions.md\n")
	b.WriteString("- Read the current task folder before working\n")
	if len(chain) > 0 {
		b.WriteString("- Read the parent task chain before changing code\n")
	}
	b.WriteString("- Do not create task folders manually under .tasker/tasks; use `tasker new`, `tasker add`, or `tasker import`\n")
	b.WriteString("- Update declaration.md, result.md, and status.json for important progress\n\n")

	b.WriteString("Relevant files:\n")
	for _, path := range currentWorkspaceFiles(chain, task) {
		b.WriteString(fmt.Sprintf("- %s\n", relativePath(root, path)))
	}
	b.WriteString("\n")

	b.WriteString("References:\n")
	if len(chain) > 0 {
		b.WriteString(fmt.Sprintf("- Parent task: %s\n", chain[len(chain)-1].Meta.ID))
	} else {
		b.WriteString("- none\n")
	}
	b.WriteString("\n")

	b.WriteString("Expected output:\n")
	for _, line := range extractGoalLines(task.TaskFile) {
		b.WriteString(fmt.Sprintf("- %s\n", line))
	}

	return b.String()
}

func renderCurrentFilesMarkdown(root string, chain []Task, task *Task) string {
	var b strings.Builder
	b.WriteString("# Relevant Files\n")
	for _, path := range currentWorkspaceFiles(chain, task) {
		b.WriteString(fmt.Sprintf("- %s\n", relativePath(root, path)))
	}
	return b.String()
}

func currentWorkspaceFiles(chain []Task, task *Task) []string {
	files := []string{
		filepath.Join(filepath.Dir(task.Path), "..", ".."),
	}
	_ = files

	paths := []string{
		task.TaskFile,
		task.InstructionsFile,
		task.DeclarationFile,
		task.ResultFile,
		task.MetaFile,
		filepath.Join(task.Path, "status.json"),
		filepath.Join(task.Path, "sessions", "index.json"),
	}

	for _, parent := range chain {
		paths = append(paths,
			parent.TaskFile,
			parent.InstructionsFile,
			parent.DeclarationFile,
			parent.ResultFile,
			parent.MetaFile,
			filepath.Join(parent.Path, "status.json"),
			filepath.Join(parent.Path, "sessions", "index.json"),
		)
	}

	return uniqueStrings(paths)
}

func buildCurrentContext(root string, chain []Task, task *Task, input CurrentWorkspaceInput) map[string]any {
	parentIDs := make([]string, 0, len(chain))
	parentTasks := make([]map[string]any, 0, len(chain))
	for _, parent := range chain {
		parentIDs = append(parentIDs, parent.Meta.ID)
		parentTasks = append(parentTasks, map[string]any{
			"id":    parent.Meta.ID,
			"title": parent.Meta.Title,
			"type":  parent.Meta.Type,
			"path":  relativePath(root, parent.Path),
		})
	}

	context := map[string]any{
		"current_task_id": task.Meta.ID,
		"task_id":         task.Meta.ID,
		"workspace_root":  root,
		"generated_at":    time.Now().Format(time.RFC3339),
		"current_task": map[string]any{
			"id":    task.Meta.ID,
			"title": task.Meta.Title,
			"type":  task.Meta.Type,
			"slug":  task.Meta.Slug,
			"path":  relativePath(root, task.Path),
		},
		"parent_task_ids": parentIDs,
		"parent_tasks":    parentTasks,
	}

	if input.Branch != "" {
		context["git"] = map[string]any{
			"branch": input.Branch,
		}
	}

	return context
}

func extractGoalLines(taskFile string) []string {
	data, err := os.ReadFile(taskFile)
	if err != nil {
		return []string{"See task.md"}
	}

	lines := strings.Split(string(data), "\n")
	inGoal := false
	collected := make([]string, 0, 4)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Goal" {
			inGoal = true
			continue
		}
		if inGoal && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if inGoal && trimmed != "" {
			collected = append(collected, trimmed)
		}
	}

	if len(collected) == 0 {
		return []string{"See task.md"}
	}
	return collected
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.Clean(rel)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func CurrentContext(root string) (map[string]any, error) {
	path := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
