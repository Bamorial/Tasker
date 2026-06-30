package tasker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ImportTaskInput struct {
	ParentID string
}

type TaskImportDocument struct {
	CommentValidTaskTypes string           `json:"_comment_valid_task_types,omitempty"`
	Tasks                 []TaskImportSpec `json:"tasks"`
}

type TaskImportSpec struct {
	CommentType  string           `json:"_comment_type,omitempty"`
	Title        string           `json:"title"`
	Type         string           `json:"type,omitempty"`
	Body         string           `json:"body,omitempty"`
	Instructions string           `json:"instructions,omitempty"`
	Declaration  string           `json:"declaration,omitempty"`
	Result       string           `json:"result,omitempty"`
	Context      json.RawMessage  `json:"context,omitempty"`
	Subtasks     []TaskImportSpec `json:"subtasks,omitempty"`
}

type ImportTasksResult struct {
	Created []*CreatedTask
	Primary *CreatedTask
}

func ImportsDir(root string) string {
	return filepath.Join(root, TaskerDirName, "imports")
}

func ImportTemplatePath(root string) string {
	return filepath.Join(root, TaskerDirName, "templates", "import-tasks.json")
}

func CreateImportTemplateCopy(root string) (string, error) {
	templatePath := ImportTemplatePath(root)
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", err
	}

	importsDir := ImportsDir(root)
	if err := os.MkdirAll(importsDir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("import-%s.json", time.Now().Format("20060102-150405"))
	path := filepath.Join(importsDir, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}

	return path, nil
}

func LatestImportPath(root string) (string, error) {
	importsDir := ImportsDir(root)
	entries, err := os.ReadDir(importsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no import files found; run `tasker import template` first")
		}
		return "", err
	}

	type importFile struct {
		path    string
		modTime time.Time
	}

	files := make([]importFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(importsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		files = append(files, importFile{path: path, modTime: info.ModTime()})
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no import files found; run `tasker import template` first")
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].path > files[j].path
		}
		return files[i].modTime.After(files[j].modTime)
	})

	return files[0].path, nil
}

func ImportTasks(root, importPath string, input ImportTaskInput) (*ImportTasksResult, error) {
	data, err := os.ReadFile(importPath)
	if err != nil {
		return nil, err
	}

	doc, err := ParseTaskImportDocument(data)
	if err != nil {
		return nil, err
	}

	result := &ImportTasksResult{
		Created: make([]*CreatedTask, 0),
	}

	parentID := strings.TrimSpace(input.ParentID)
	for i := range doc.Tasks {
		created, err := importTaskSpec(root, doc.Tasks[i], parentID, result)
		if err != nil {
			return nil, err
		}
		if result.Primary == nil {
			result.Primary = created
		}
	}

	return result, nil
}

func ParseTaskImportDocument(content []byte) (*TaskImportDocument, error) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	var doc TaskImportDocument
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse import JSON: %w", err)
	}

	if len(doc.Tasks) == 0 {
		return nil, fmt.Errorf("import document must include at least one task")
	}

	for i := range doc.Tasks {
		if err := validateTaskImportSpec(doc.Tasks[i], fmt.Sprintf("tasks[%d]", i)); err != nil {
			return nil, err
		}
	}

	return &doc, nil
}

func validateTaskImportSpec(spec TaskImportSpec, path string) error {
	if strings.TrimSpace(spec.Title) == "" {
		return fmt.Errorf("%s.title is required", path)
	}

	taskType := strings.TrimSpace(strings.ToLower(spec.Type))
	if taskType != "" {
		if _, ok := validTaskTypes[taskType]; !ok {
			return fmt.Errorf("%s.type is invalid: %q", path, spec.Type)
		}
	}

	if len(spec.Context) > 0 && !json.Valid(spec.Context) {
		return fmt.Errorf("%s.context must be valid JSON", path)
	}

	for i := range spec.Subtasks {
		if err := validateTaskImportSpec(spec.Subtasks[i], fmt.Sprintf("%s.subtasks[%d]", path, i)); err != nil {
			return err
		}
	}

	return nil
}

func importTaskSpec(root string, spec TaskImportSpec, parentID string, result *ImportTasksResult) (*CreatedTask, error) {
	created, err := CreateTask(root, CreateTaskInput{
		Title:    spec.Title,
		Type:     spec.Type,
		ParentID: parentID,
	})
	if err != nil {
		return nil, err
	}

	if err := writeImportedTaskFiles(created.Path, spec); err != nil {
		return nil, err
	}

	result.Created = append(result.Created, created)

	for i := range spec.Subtasks {
		if _, err := importTaskSpec(root, spec.Subtasks[i], created.ID, result); err != nil {
			return nil, err
		}
	}

	return created, nil
}

func writeImportedTaskFiles(taskPath string, spec TaskImportSpec) error {
	if err := writeImportedTextFile(filepath.Join(taskPath, "task.md"), spec.Body); err != nil {
		return err
	}
	if err := writeImportedTextFile(filepath.Join(taskPath, "instructions.md"), spec.Instructions); err != nil {
		return err
	}
	if err := writeImportedTextFile(filepath.Join(taskPath, "declaration.md"), spec.Declaration); err != nil {
		return err
	}
	if err := writeImportedTextFile(filepath.Join(taskPath, "result.md"), spec.Result); err != nil {
		return err
	}
	if len(spec.Context) > 0 {
		if err := writeImportedJSONFile(filepath.Join(taskPath, "context.json"), spec.Context); err != nil {
			return err
		}
	}
	return nil
}

func writeImportedTextFile(path, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeImportedJSONFile(path string, content json.RawMessage) error {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil
	}

	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return err
	}
	return writeJSON(path, value)
}
