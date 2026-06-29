package tasker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type StatusFormatOptions struct {
	Color bool
}

var validTaskTypes = map[string]struct{}{
	"feature":       {},
	"bug":           {},
	"research":      {},
	"decision":      {},
	"documentation": {},
	"review":        {},
}

func ValidTaskTypes() []string {
	types := make([]string, 0, len(validTaskTypes))
	for taskType := range validTaskTypes {
		types = append(types, taskType)
	}
	slices.Sort(types)
	return types
}

type CreateTaskInput struct {
	Title    string
	Type     string
	ParentID string
}

type CreatedTask struct {
	ID       string
	Path     string
	TaskFile string
}

type TaskMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	Type      string `json:"type"`
	ParentID  string `json:"parent_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

type TaskStatus struct {
	Status  string `json:"status"`
	Agent   string `json:"agent"`
	Started string `json:"started"`
}

type Task struct {
	Meta             TaskMeta
	Status           TaskStatus
	Path             string
	MetaFile         string
	TaskFile         string
	InstructionsFile string
	DeclarationFile  string
	ResultFile       string
}

type UpdateTaskMetaInput struct {
	Title string
	Type  string
}

func DeleteTask(root, id string, recursive bool) error {
	task, err := GetTask(root, id)
	if err != nil {
		return err
	}

	hasChildren, err := taskHasChildren(task.Path)
	if err != nil {
		return err
	}
	if hasChildren && !recursive {
		return fmt.Errorf("task %s has child tasks; rerun with --recursive", id)
	}

	return os.RemoveAll(task.Path)
}

func CreateTask(root string, input CreateTaskInput) (*CreatedTask, error) {
	taskType := strings.TrimSpace(strings.ToLower(input.Type))
	if taskType == "" {
		taskType = "feature"
	}
	if _, ok := validTaskTypes[taskType]; !ok {
		return nil, fmt.Errorf("invalid task type %q", input.Type)
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "Untitled task"
	}

	baseDir := filepath.Join(root, TaskerDirName, "tasks")
	parentID := strings.TrimSpace(input.ParentID)
	if parentID != "" {
		parent, err := GetTask(root, parentID)
		if err != nil {
			return nil, err
		}
		baseDir = filepath.Join(parent.Path, "children")
	}

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}

	id, err := nextTaskID(root)
	if err != nil {
		return nil, err
	}

	slug := slugify(title)
	taskDirName := id + "-" + slug
	taskPath := filepath.Join(baseDir, taskDirName)

	if err := os.MkdirAll(filepath.Join(taskPath, "children"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(taskPath, "sessions"), 0o755); err != nil {
		return nil, err
	}

	meta := TaskMeta{
		ID:        id,
		Title:     title,
		Slug:      slug,
		Type:      taskType,
		ParentID:  parentID,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	status := TaskStatus{
		Status:  "NEW",
		Agent:   "unknown",
		Started: time.Now().Format(time.RFC3339),
	}

	files := map[string]string{
		filepath.Join(taskPath, "task.md"):         taskMarkdown(meta),
		filepath.Join(taskPath, "instructions.md"): "# Task Instructions\n\nAdd task-specific rules here.\n",
		filepath.Join(taskPath, "declaration.md"):  "# Declaration\n\nStatus:\n\nUnderstanding:\n\nCompleted:\n\nFiles:\n\nDecisions:\n\nRemaining:\n\nNext agent:\n",
		filepath.Join(taskPath, "result.md"):       "# Result\n\nSummary:\n",
		filepath.Join(taskPath, "context.json"):    "{}\n",
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, err
		}
	}

	if err := writeJSON(filepath.Join(taskPath, "meta.json"), meta); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(taskPath, "status.json"), status); err != nil {
		return nil, err
	}

	return &CreatedTask{
		ID:       meta.ID,
		Path:     taskPath,
		TaskFile: filepath.Join(taskPath, "task.md"),
	}, nil
}

func TaskDocumentPath(taskPath, target string) (string, error) {
	switch normalizeOpenTarget(target) {
	case "task":
		return filepath.Join(taskPath, "task.md"), nil
	case "instructions":
		return filepath.Join(taskPath, "instructions.md"), nil
	case "declaration":
		return filepath.Join(taskPath, "declaration.md"), nil
	case "result":
		return filepath.Join(taskPath, "result.md"), nil
	case "meta":
		return filepath.Join(taskPath, "meta.json"), nil
	default:
		return "", fmt.Errorf("invalid open target %q", target)
	}
}

func GetTask(root, id string) (*Task, error) {
	tasks, err := loadTasks(root)
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.Meta.ID == id {
			return &task, nil
		}
	}

	return nil, fmt.Errorf("task %s not found", id)
}

func TaskTree(root string) ([]string, error) {
	tasksRoot := filepath.Join(root, TaskerDirName, "tasks")
	lines := make([]string, 0)
	entries, err := os.ReadDir(tasksRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"No tasks."}, nil
		}
		return nil, err
	}

	paths := collectTaskDirs(tasksRoot, entries)
	sort.Strings(paths)
	if len(paths) == 0 {
		return []string{"No tasks."}, nil
	}

	for _, path := range paths {
		if filepath.Dir(path) != tasksRoot {
			continue
		}
		if err := appendTaskTree(path, 0, &lines); err != nil {
			return nil, err
		}
	}
	return lines, nil
}

func TaskStatuses(root string) ([]string, error) {
	return TaskStatusesStyled(root, StatusFormatOptions{})
}

func TaskStatusesStyled(root string, opts StatusFormatOptions) ([]string, error) {
	tasks, err := loadTasks(root)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return []string{"No tasks."}, nil
	}

	sort.Slice(tasks, func(i, j int) bool {
		return taskSortKey(tasks[i].Meta.ID, tasks[i].Path) < taskSortKey(tasks[j].Meta.ID, tasks[j].Path)
	})

	palette := newStatusPalette(opts.Color)
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		lines = append(lines, formatTaskSummaryLine(task, "", palette))
	}
	return lines, nil
}

func TaskStatusDetails(root, id string) ([]string, error) {
	return TaskStatusDetailsStyled(root, id, StatusFormatOptions{})
}

func TaskStatusDetailsStyled(root, id string, opts StatusFormatOptions) ([]string, error) {
	task, err := GetTask(root, id)
	if err != nil {
		return nil, err
	}

	palette := newStatusPalette(opts.Color)
	lines := []string{
		palette.heading(fmt.Sprintf("Task %s: %s", task.Meta.ID, task.Meta.Title)),
		formatDetailField("Status", palette.statusBadge(task.Status.Status), palette),
		formatDetailField("Type", task.Meta.Type, palette),
		formatDetailField("Agent", task.Status.Agent, palette),
		formatDetailField("Created", task.Meta.CreatedAt, palette),
		formatDetailField("Started", task.Status.Started, palette),
		formatDetailField("Path", task.Path, palette),
	}

	if task.Meta.ParentID != "" {
		lines = append(lines, formatDetailField("Parent", task.Meta.ParentID, palette))
	}

	children, err := childTasks(task.Path)
	if err != nil {
		return nil, err
	}

	lines = append(lines, "")
	lines = append(lines, palette.section("Subtasks"))
	if len(children) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, child := range children {
			appendTaskStatusTreeStyled(child, 1, &lines, palette)
		}
	}

	lines = append(lines, "")
	lines = append(lines, taskNotes(task, palette)...)

	return lines, nil
}

func loadTasks(root string) ([]Task, error) {
	tasksRoot := filepath.Join(root, TaskerDirName, "tasks")
	var tasks []Task

	err := filepath.WalkDir(tasksRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() || path == tasksRoot || filepath.Base(path) == "children" || filepath.Base(path) == "sessions" {
			return nil
		}

		metaPath := filepath.Join(path, "meta.json")
		if _, err := os.Stat(metaPath); err != nil {
			return nil
		}

		var meta TaskMeta
		if err := readJSON(metaPath, &meta); err != nil {
			return err
		}

		var status TaskStatus
		if err := readJSON(filepath.Join(path, "status.json"), &status); err != nil {
			return err
		}

		tasks = append(tasks, Task{
			Meta:             meta,
			Status:           status,
			Path:             path,
			MetaFile:         metaPath,
			TaskFile:         filepath.Join(path, "task.md"),
			InstructionsFile: filepath.Join(path, "instructions.md"),
			DeclarationFile:  filepath.Join(path, "declaration.md"),
			ResultFile:       filepath.Join(path, "result.md"),
		})
		return nil
	})
	if os.IsNotExist(err) {
		return []Task{}, nil
	}
	return tasks, err
}

func appendTaskTree(path string, depth int, lines *[]string) error {
	var meta TaskMeta
	if err := readJSON(filepath.Join(path, "meta.json"), &meta); err != nil {
		return err
	}

	prefix := strings.Repeat("  ", depth)
	*lines = append(*lines, fmt.Sprintf("%s%s %s [%s]", prefix, meta.ID, meta.Title, meta.Type))

	childrenDir := filepath.Join(path, "children")
	entries, err := os.ReadDir(childrenDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	childPaths := collectTaskDirs(childrenDir, entries)
	sort.Strings(childPaths)
	for _, childPath := range childPaths {
		if err := appendTaskTree(childPath, depth+1, lines); err != nil {
			return err
		}
	}
	return nil
}

func taskHasChildren(taskPath string) (bool, error) {
	childrenDir := filepath.Join(taskPath, "children")
	entries, err := os.ReadDir(childrenDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "0") {
			return true, nil
		}
	}

	return false, nil
}

func collectTaskDirs(base string, entries []os.DirEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "0") {
			paths = append(paths, filepath.Join(base, entry.Name()))
		}
	}
	return paths
}

func childTasks(taskPath string) ([]Task, error) {
	childrenDir := filepath.Join(taskPath, "children")
	entries, err := os.ReadDir(childrenDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, err
	}

	paths := collectTaskDirs(childrenDir, entries)
	sort.Strings(paths)

	children := make([]Task, 0, len(paths))
	for _, path := range paths {
		var meta TaskMeta
		if err := readJSON(filepath.Join(path, "meta.json"), &meta); err != nil {
			return nil, err
		}

		var status TaskStatus
		if err := readJSON(filepath.Join(path, "status.json"), &status); err != nil {
			return nil, err
		}

		children = append(children, Task{
			Meta:             meta,
			Status:           status,
			Path:             path,
			MetaFile:         filepath.Join(path, "meta.json"),
			TaskFile:         filepath.Join(path, "task.md"),
			InstructionsFile: filepath.Join(path, "instructions.md"),
			DeclarationFile:  filepath.Join(path, "declaration.md"),
			ResultFile:       filepath.Join(path, "result.md"),
		})
	}

	return children, nil
}

func appendTaskStatusTree(task Task, depth int, lines *[]string) {
	appendTaskStatusTreeStyled(task, depth, lines, newStatusPalette(false))
}

func appendTaskStatusTreeStyled(task Task, depth int, lines *[]string, palette statusPalette) {
	prefix := strings.Repeat("  ", depth)
	*lines = append(*lines, formatTaskSummaryLine(task, prefix, palette))

	children, err := childTasks(task.Path)
	if err != nil {
		*lines = append(*lines, fmt.Sprintf("%s  [error loading subtasks: %v]", prefix, err))
		return
	}

	for _, child := range children {
		appendTaskStatusTreeStyled(child, depth+1, lines, palette)
	}
}

func taskNotes(task *Task, palette statusPalette) []string {
	sections := []struct {
		title string
		path  string
	}{
		{title: "Goal", path: task.TaskFile},
		{title: "Instructions", path: task.InstructionsFile},
		{title: "Declaration", path: task.DeclarationFile},
		{title: "Result", path: task.ResultFile},
	}

	lines := []string{palette.section("Notes")}
	for _, section := range sections {
		content, err := readTrimmedFile(section.path)
		if err != nil {
			lines = append(lines, fmt.Sprintf("  %s %s", palette.label(section.title), palette.subtle(fmt.Sprintf("[error: %v]", err))))
			continue
		}
		if content == "" {
			lines = append(lines, fmt.Sprintf("  %s %s", palette.label(section.title), palette.subtle("none")))
			continue
		}

		lines = append(lines, fmt.Sprintf("  %s", palette.label(section.title)))
		for _, line := range strings.Split(content, "\n") {
			lines = append(lines, "    "+line)
		}
	}

	return lines
}

func readTrimmedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", nil
	}

	return content, nil
}

func nextTaskID(root string) (string, error) {
	tasks, err := loadTasks(root)
	if err != nil {
		return "", err
	}

	maxID := 0
	for _, task := range tasks {
		n, err := strconv.Atoi(task.Meta.ID)
		if err == nil && n > maxID {
			maxID = n
		}
	}

	return fmt.Sprintf("%03d", maxID+1), nil
}

func slugify(input string) string {
	var b strings.Builder
	lastDash := false

	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteRune('-')
			lastDash = true
		}
	}

	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "task"
	}
	return slug
}

func taskMarkdown(meta TaskMeta) string {
	return fmt.Sprintf(`# %s

ID: %s
Type: %s
Created: %s

## Goal

Describe the goal and requirements.
`, meta.Title, meta.ID, meta.Type, meta.CreatedAt)
}

func InferParentTaskID(root, start string) (string, error) {
	if id, err := inferParentTaskIDFromPath(root, start); err == nil {
		return id, nil
	}

	if id, err := inferParentTaskIDFromContext(root); err == nil {
		return id, nil
	}

	if id, err := inferParentTaskIDFromWorkspace(root); err == nil {
		return id, nil
	}

	return "", fmt.Errorf("could not infer parent task; run inside a task directory or pass --parent <id>")
}

func UpdateTaskMeta(root, id string, input UpdateTaskMetaInput) (*Task, error) {
	task, err := GetTask(root, id)
	if err != nil {
		return nil, err
	}

	meta := task.Meta
	changed := false

	title := strings.TrimSpace(input.Title)
	if title != "" && title != meta.Title {
		meta.Title = title
		meta.Slug = slugify(title)
		changed = true
	}

	taskType := strings.TrimSpace(strings.ToLower(input.Type))
	if taskType != "" {
		if _, ok := validTaskTypes[taskType]; !ok {
			return nil, fmt.Errorf("invalid task type %q", input.Type)
		}
		if taskType != meta.Type {
			meta.Type = taskType
			changed = true
		}
	}

	if !changed {
		return task, nil
	}

	newPath := task.Path
	newDirName := meta.ID + "-" + meta.Slug
	if filepath.Base(task.Path) != newDirName {
		newPath = filepath.Join(filepath.Dir(task.Path), newDirName)
		if _, err := os.Stat(newPath); err == nil {
			return nil, fmt.Errorf("task path already exists: %s", newPath)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if err := os.Rename(task.Path, newPath); err != nil {
			return nil, err
		}
	}

	if err := writeJSON(filepath.Join(newPath, "meta.json"), meta); err != nil {
		return nil, err
	}

	return GetTask(root, id)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func taskSortKey(id, path string) string {
	return path + ":" + id
}

type statusPalette struct {
	enabled bool
}

func newStatusPalette(enabled bool) statusPalette {
	return statusPalette{enabled: enabled}
}

func (p statusPalette) heading(value string) string {
	return p.wrap("1;36", value)
}

func (p statusPalette) section(value string) string {
	return p.wrap("1", value)
}

func (p statusPalette) label(value string) string {
	return p.wrap("1", value)
}

func (p statusPalette) subtle(value string) string {
	return p.wrap("2", value)
}

func (p statusPalette) statusBadge(status string) string {
	badge := "[" + status + "]"
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "NEW":
		return p.wrap("1;34", badge)
	case "IN_PROGRESS":
		return p.wrap("1;33", badge)
	case "DONE":
		return p.wrap("1;32", badge)
	case "BLOCKED":
		return p.wrap("1;31", badge)
	case "REVIEW":
		return p.wrap("1;35", badge)
	default:
		return p.wrap("1;36", badge)
	}
}

func (p statusPalette) wrap(code, value string) string {
	if !p.enabled || value == "" {
		return value
	}
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func formatTaskSummaryLine(task Task, prefix string, palette statusPalette) string {
	return fmt.Sprintf(
		"%s%s  %-18s %s %s",
		prefix,
		task.Meta.ID,
		palette.statusBadge(task.Status.Status),
		task.Meta.Title,
		palette.subtle("("+task.Meta.Type+")"),
	)
}

func formatDetailField(label, value string, palette statusPalette) string {
	return fmt.Sprintf("%-8s %s", palette.label(label), value)
}

func normalizeOpenTarget(target string) string {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case "", "task", "goal":
		return "task"
	case "instructions", "instruction":
		return "instructions"
	case "declaration", "handoff":
		return "declaration"
	case "result", "report":
		return "result"
	case "meta", "metadata":
		return "meta"
	default:
		return strings.TrimSpace(strings.ToLower(target))
	}
}

func inferParentTaskIDFromPath(root, start string) (string, error) {
	tasks, err := loadTasks(root)
	if err != nil {
		return "", err
	}

	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	var best *Task
	for i := range tasks {
		task := tasks[i]
		if !pathContains(task.Path, absStart) {
			continue
		}
		if best == nil || len(task.Path) > len(best.Path) {
			best = &task
		}
	}

	if best == nil {
		return "", fmt.Errorf("no task in current path")
	}
	return best.Meta.ID, nil
}

func inferParentTaskIDFromContext(root string) (string, error) {
	path := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}

	for _, key := range []string{"current_task_id", "task_id"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if id := strings.TrimSpace(fmt.Sprint(value)); id != "" && id != "<nil>" {
			return id, nil
		}
	}

	if nested, ok := payload["current_task"].(map[string]any); ok {
		if id := strings.TrimSpace(fmt.Sprint(nested["id"])); id != "" && id != "<nil>" {
			return id, nil
		}
	}

	return "", fmt.Errorf("no current task in context")
}

func inferParentTaskIDFromWorkspace(root string) (string, error) {
	path := filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "current task:") {
			continue
		}

		fields := strings.Fields(line)
		for _, field := range fields {
			field = strings.Trim(field, "[]():")
			if len(field) == 3 {
				if _, err := strconv.Atoi(field); err == nil {
					return field, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no current task in workspace")
}

func pathContains(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	if base == target {
		return true
	}
	return strings.HasPrefix(target, base+string(os.PathSeparator))
}
