package tasker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type TaskTreeItem struct {
	Task  Task
	Depth int
}

type WorkspaceSnapshot struct {
	Root         string
	Config       Config
	Tasks        []Task
	Tree         []TaskTreeItem
	StatusCounts map[string]int
	Current      CurrentWorkspaceSnapshot
}

type CurrentWorkspaceSnapshot struct {
	Task        *Task
	ParentChain []Task
	Context     map[string]any
	Branch      string
}

func ListTasks(root string) ([]Task, error) {
	tasks, err := loadTasks(root)
	if err != nil {
		return nil, err
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Meta.ID < tasks[j].Meta.ID
	})
	return tasks, nil
}

func TaskParentChain(root string, task *Task) ([]Task, error) {
	return taskParentChain(root, task)
}

func TaskChildren(task *Task) ([]Task, error) {
	children, err := childTasks(task.Path)
	if err != nil {
		return nil, err
	}

	sort.Slice(children, func(i, j int) bool {
		return children[i].Meta.ID < children[j].Meta.ID
	})
	return children, nil
}

func LoadTaskTree(root string) ([]TaskTreeItem, error) {
	tasks, err := ListTasks(root)
	if err != nil {
		return nil, err
	}

	byParent := make(map[string][]Task)
	for _, task := range tasks {
		byParent[task.Meta.ParentID] = append(byParent[task.Meta.ParentID], task)
	}
	for parentID := range byParent {
		sort.Slice(byParent[parentID], func(i, j int) bool {
			return byParent[parentID][i].Meta.ID < byParent[parentID][j].Meta.ID
		})
	}

	items := make([]TaskTreeItem, 0, len(tasks))
	var appendChildren func(parentID string, depth int)
	appendChildren = func(parentID string, depth int) {
		for _, task := range byParent[parentID] {
			items = append(items, TaskTreeItem{
				Task:  task,
				Depth: depth,
			})
			appendChildren(task.Meta.ID, depth+1)
		}
	}
	appendChildren("", 0)

	return items, nil
}

func LoadWorkspaceSnapshot(root string) (*WorkspaceSnapshot, error) {
	cfg, err := LoadConfig(root)
	if err != nil {
		return nil, err
	}

	tasks, err := ListTasks(root)
	if err != nil {
		return nil, err
	}

	tree, err := LoadTaskTree(root)
	if err != nil {
		return nil, err
	}

	current, err := ReadCurrentWorkspaceSnapshot(root)
	if err != nil {
		return nil, err
	}

	return &WorkspaceSnapshot{
		Root:         root,
		Config:       cfg,
		Tasks:        tasks,
		Tree:         tree,
		StatusCounts: CountTasksByStatus(tasks),
		Current:      *current,
	}, nil
}

func ReadCurrentWorkspaceSnapshot(root string) (*CurrentWorkspaceSnapshot, error) {
	context := map[string]any{}
	contextPath := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")
	if _, err := os.Stat(contextPath); err == nil {
		payload, err := CurrentContext(root)
		if err != nil {
			return nil, err
		}
		context = payload
	}

	snapshot := &CurrentWorkspaceSnapshot{
		Context: context,
	}

	taskID := strings.TrimSpace(stringValue(context["current_task_id"]))
	if taskID == "" {
		taskID = strings.TrimSpace(stringValue(context["task_id"]))
	}
	if taskID == "" {
		return snapshot, nil
	}

	task, err := GetTask(root, taskID)
	if err != nil {
		return nil, err
	}
	chain, err := TaskParentChain(root, task)
	if err != nil {
		return nil, err
	}

	snapshot.Task = task
	snapshot.ParentChain = chain
	if gitPayload, ok := context["git"].(map[string]any); ok {
		snapshot.Branch = strings.TrimSpace(stringValue(gitPayload["branch"]))
	}
	return snapshot, nil
}

func CountTasksByStatus(tasks []Task) map[string]int {
	counts := make(map[string]int, len(validTaskStatuses))
	for status := range validTaskStatuses {
		counts[status] = 0
	}
	for _, task := range tasks {
		counts[canonicalTaskStatus(task.Status.Status)]++
	}
	return counts
}

func TaskSummary(task Task) string {
	return fmt.Sprintf("%s %s", task.Meta.ID, task.Meta.Title)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
