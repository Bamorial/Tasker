package tasker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetTaskIncludesInstructionsFile(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Docs", Type: "documentation"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	want := filepath.Join(created.Path, "instructions.md")
	if task.InstructionsFile != want {
		t.Fatalf("expected instructions file %s, got %s", want, task.InstructionsFile)
	}

	if task.DeclarationFile != filepath.Join(created.Path, "declaration.md") {
		t.Fatalf("unexpected declaration file path: %s", task.DeclarationFile)
	}

	if task.ResultFile != filepath.Join(created.Path, "result.md") {
		t.Fatalf("unexpected result file path: %s", task.ResultFile)
	}

	if _, err := os.Stat(task.InstructionsFile); err != nil {
		t.Fatalf("expected instructions file to exist: %v", err)
	}
}

func TestDeleteLeafTask(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Leaf", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := DeleteTask(root, created.ID, false); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Fatalf("expected task path to be removed, got err=%v", err)
	}
}

func TestTaskStatusesIncludesAllTasks(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	first, err := CreateTask(root, CreateTaskInput{Title: "Alpha", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}

	second, err := CreateTask(root, CreateTaskInput{Title: "Beta", Type: "bug"})
	if err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}

	rows, err := TaskStatuses(root)
	if err != nil {
		t.Fatalf("TaskStatuses: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if !strings.Contains(rows[0], first.ID) || !strings.Contains(rows[0], "[NEW]") || !strings.Contains(rows[0], "Alpha") {
		t.Fatalf("expected first row to include task %s, got %q", first.ID, rows[0])
	}

	if !strings.Contains(rows[1], second.ID) || !strings.Contains(rows[1], "[NEW]") || !strings.Contains(rows[1], "Beta") {
		t.Fatalf("expected second row to include task %s, got %q", second.ID, rows[1])
	}
}

func TestTaskStatusesShowsTreeAndTaskMetadata(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	child, err := CreateTask(root, CreateTaskInput{
		Title:    "Child",
		Type:     "documentation",
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	if err := writeJSON(filepath.Join(parent.Path, "status.json"), TaskStatus{
		Status:  "IN_PROGRESS",
		Agent:   "planner",
		Started: "2026-06-29T20:00:00+03:00",
	}); err != nil {
		t.Fatalf("write parent status: %v", err)
	}

	if err := writeJSON(filepath.Join(child.Path, "status.json"), TaskStatus{
		Status:  "DONE",
		Agent:   "worker",
		Started: "2026-06-29T20:05:00+03:00",
	}); err != nil {
		t.Fatalf("write child status: %v", err)
	}

	rows, err := TaskStatuses(root)
	if err != nil {
		t.Fatalf("TaskStatuses: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if !strings.Contains(rows[0], parent.ID) ||
		!strings.Contains(rows[0], "[IN_PROGRESS]") ||
		!strings.Contains(rows[0], "Parent") ||
		!strings.Contains(rows[0], "agent=planner") ||
		!strings.Contains(rows[0], "1 child") {
		t.Fatalf("expected parent row to include metadata, got %q", rows[0])
	}

	if !strings.HasPrefix(rows[1], "  "+child.ID) ||
		!strings.Contains(rows[1], "[DONE]") ||
		!strings.Contains(rows[1], "Child") ||
		!strings.Contains(rows[1], "agent=worker") {
		t.Fatalf("expected child row to be indented subtree output, got %q", rows[1])
	}
}

func TestTaskStatusDetailsIncludesSubtasks(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	child, err := CreateTask(root, CreateTaskInput{
		Title:    "Child",
		Type:     "documentation",
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	grandchild, err := CreateTask(root, CreateTaskInput{
		Title:    "Grandchild",
		Type:     "review",
		ParentID: child.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask grandchild: %v", err)
	}

	if err := writeJSON(filepath.Join(child.Path, "status.json"), TaskStatus{
		Status:  "IN_PROGRESS",
		Agent:   "worker",
		Started: "2026-06-29T19:00:00+03:00",
	}); err != nil {
		t.Fatalf("write child status: %v", err)
	}

	if err := writeJSON(filepath.Join(grandchild.Path, "status.json"), TaskStatus{
		Status:  "DONE",
		Agent:   "reviewer",
		Started: "2026-06-29T19:05:00+03:00",
	}); err != nil {
		t.Fatalf("write grandchild status: %v", err)
	}

	if err := os.WriteFile(parent.TaskFile, []byte("# Parent\n\n## Goal\n\nTrack work.\n"), 0o644); err != nil {
		t.Fatalf("write parent task file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(parent.Path, "instructions.md"), []byte("# Task Instructions\n\nUse care.\n"), 0o644); err != nil {
		t.Fatalf("write parent instructions file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(parent.Path, "declaration.md"), []byte("# Declaration\n\nStatus:\nWorking on it.\n"), 0o644); err != nil {
		t.Fatalf("write parent declaration file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(parent.Path, "result.md"), []byte("# Result\n\nSummary:\nPending review.\n"), 0o644); err != nil {
		t.Fatalf("write parent result file: %v", err)
	}

	rows, err := TaskStatusDetails(root, parent.ID)
	if err != nil {
		t.Fatalf("TaskStatusDetails: %v", err)
	}

	output := strings.Join(rows, "\n")
	for _, want := range []string{
		fmt.Sprintf("Task %s: Parent", parent.ID),
		"Status   [NEW]",
		"Subtasks",
		fmt.Sprintf("  %s  [IN_PROGRESS]", child.ID),
		fmt.Sprintf("    %s  [DONE]", grandchild.ID),
		"Notes",
		"  Goal",
		"    ## Goal",
		"  Instructions",
		"    Use care.",
		"  Declaration",
		"    Working on it.",
		"  Result",
		"    Pending review.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected detailed status to include %q, got:\n%s", want, output)
		}
	}
}

func TestTaskStatusesStyledIncludesANSIWhenEnabled(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	if _, err := CreateTask(root, CreateTaskInput{Title: "Alpha", Type: "feature"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := TaskStatusesStyled(root, StatusFormatOptions{Color: true})
	if err != nil {
		t.Fatalf("TaskStatusesStyled: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	if !strings.Contains(rows[0], "\x1b[1;34m[NEW]\x1b[0m") {
		t.Fatalf("expected styled row to include ANSI status badge, got %q", rows[0])
	}

	if !strings.Contains(rows[0], "(feature | agent=unknown)") {
		t.Fatalf("expected styled row to include task metadata, got %q", rows[0])
	}
}

func TestValidTaskTypesSorted(t *testing.T) {
	got := ValidTaskTypes()
	want := []string{"bug", "decision", "documentation", "feature", "research", "review"}

	if len(got) != len(want) {
		t.Fatalf("expected %d task types, got %d", len(want), len(got))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected task type %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestDeleteParentRequiresRecursive(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	if _, err := CreateTask(root, CreateTaskInput{
		Title:    "Child",
		Type:     "feature",
		ParentID: parent.ID,
	}); err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	if err := DeleteTask(root, parent.ID, false); err == nil {
		t.Fatal("expected delete without --recursive to fail")
	}

	if _, err := os.Stat(parent.Path); err != nil {
		t.Fatalf("expected parent task to remain, got err=%v", err)
	}
}

func TestDeleteParentRecursive(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	child, err := CreateTask(root, CreateTaskInput{
		Title:    "Child",
		Type:     "feature",
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	if err := DeleteTask(root, parent.ID, true); err != nil {
		t.Fatalf("DeleteTask recursive: %v", err)
	}

	for _, path := range []string{parent.Path, child.Path} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected path %s to be removed, got err=%v", path, err)
		}
	}

	tasksRoot := filepath.Join(root, TaskerDirName, "tasks")
	entries, err := os.ReadDir(tasksRoot)
	if err != nil {
		t.Fatalf("ReadDir tasks root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected tasks root to be empty, got %d entries", len(entries))
	}
}

func TestTaskDocumentPathTargets(t *testing.T) {
	taskPath := filepath.Join("/tmp", "001-example")

	tests := map[string]string{
		"task":         filepath.Join(taskPath, "task.md"),
		"instructions": filepath.Join(taskPath, "instructions.md"),
		"declaration":  filepath.Join(taskPath, "declaration.md"),
		"result":       filepath.Join(taskPath, "result.md"),
		"meta":         filepath.Join(taskPath, "meta.json"),
		"metadata":     filepath.Join(taskPath, "meta.json"),
	}

	for target, want := range tests {
		got, err := TaskDocumentPath(taskPath, target)
		if err != nil {
			t.Fatalf("TaskDocumentPath(%q): %v", target, err)
		}
		if got != want {
			t.Fatalf("TaskDocumentPath(%q) = %s, want %s", target, got, want)
		}
	}

	if _, err := TaskDocumentPath(taskPath, "unknown"); err == nil {
		t.Fatal("expected invalid target to fail")
	}
}

func TestInferParentTaskIDFromPath(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	child, err := CreateTask(root, CreateTaskInput{
		Title:    "Child",
		Type:     "feature",
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	got, err := InferParentTaskID(root, filepath.Join(child.Path, "children"))
	if err != nil {
		t.Fatalf("InferParentTaskID: %v", err)
	}
	if got != child.ID {
		t.Fatalf("expected inferred parent %s, got %s", child.ID, got)
	}
}

func TestInferParentTaskIDFromContext(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	contextPath := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")
	if err := os.WriteFile(contextPath, []byte(fmt.Sprintf("{\n  \"current_task_id\": %q\n}\n", parent.ID)), 0o644); err != nil {
		t.Fatalf("WriteFile context: %v", err)
	}

	got, err := InferParentTaskID(root, root)
	if err != nil {
		t.Fatalf("InferParentTaskID: %v", err)
	}
	if got != parent.ID {
		t.Fatalf("expected inferred parent %s, got %s", parent.ID, got)
	}
}

func TestUpdateTaskMetaRenamesTaskFolder(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Old Name", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	updated, err := UpdateTaskMeta(root, created.ID, UpdateTaskMetaInput{
		Title: "New Name",
		Type:  "review",
	})
	if err != nil {
		t.Fatalf("UpdateTaskMeta: %v", err)
	}

	if updated.Meta.Title != "New Name" {
		t.Fatalf("expected updated title, got %q", updated.Meta.Title)
	}
	if updated.Meta.Type != "review" {
		t.Fatalf("expected updated type, got %q", updated.Meta.Type)
	}
	if updated.Meta.Slug != "new-name" {
		t.Fatalf("expected updated slug, got %q", updated.Meta.Slug)
	}
	if filepath.Base(updated.Path) != created.ID+"-new-name" {
		t.Fatalf("expected renamed path, got %s", updated.Path)
	}
	if _, err := os.Stat(created.Path); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be moved, got err=%v", err)
	}
	if _, err := os.Stat(updated.MetaFile); err != nil {
		t.Fatalf("expected updated meta file to exist: %v", err)
	}
}

func TestCheckoutTaskUpdatesCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Parent Task", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	child, err := CreateTask(root, CreateTaskInput{
		Title:    "Child Task",
		Type:     "documentation",
		ParentID: parent.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	result, err := CheckoutTask(root, child.ID, CheckoutTaskInput{NoBranch: true})
	if err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}

	if result.Branch != "" {
		t.Fatalf("expected no branch, got %q", result.Branch)
	}

	workspacePath := filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md")
	data, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("ReadFile workspace: %v", err)
	}
	workspace := string(data)
	for _, want := range []string{
		fmt.Sprintf("- %s Child Task", child.ID),
		fmt.Sprintf("- %s Parent Task", parent.ID),
		"Read the parent task chain before changing code",
	} {
		if !strings.Contains(workspace, want) {
			t.Fatalf("expected workspace to include %q, got:\n%s", want, workspace)
		}
	}

	context, err := CurrentContext(root)
	if err != nil {
		t.Fatalf("CurrentContext: %v", err)
	}
	if got := fmt.Sprint(context["current_task_id"]); got != child.ID {
		t.Fatalf("expected current_task_id %s, got %s", child.ID, got)
	}
}

func TestCheckoutTaskDoesNotCreateBranchWithoutCheckoutFlag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	initializeGitRepo(t, root)

	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, TaskerDirName, "config.yaml"), []byte("editor: \"\"\ngit:\n  enabled: true\n  branch_per_task: true\n  checkout_branch: false\n  commit_per_subtask: true\n  branch_prefix: \"task\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	task, err := CreateTask(root, CreateTaskInput{Title: "Root Feature", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	result, err := CheckoutTask(root, task.ID, CheckoutTaskInput{})
	if err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}

	if result.Branch != "" {
		t.Fatalf("expected no branch, got %q", result.Branch)
	}

	repo, err := OpenGitRepo(root)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}
	gotBranch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if gotBranch != "main" {
		t.Fatalf("expected current branch main, got %s", gotBranch)
	}
}

func TestCheckoutTaskCreatesBranchWhenCheckoutFlagEnabled(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	initializeGitRepo(t, root)

	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, TaskerDirName, "config.yaml"), []byte("editor: \"\"\ngit:\n  enabled: true\n  branch_per_task: true\n  checkout_branch: true\n  commit_per_subtask: true\n  branch_prefix: \"task\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	task, err := CreateTask(root, CreateTaskInput{Title: "Root Feature", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	result, err := CheckoutTask(root, task.ID, CheckoutTaskInput{})
	if err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}

	wantBranch := "task/" + task.ID + "-root-feature"
	if result.Branch != wantBranch {
		t.Fatalf("expected branch %s, got %s", wantBranch, result.Branch)
	}

	repo, err := OpenGitRepo(root)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}
	gotBranch, err := repo.CurrentBranch()
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if gotBranch != wantBranch {
		t.Fatalf("expected current branch %s, got %s", wantBranch, gotBranch)
	}
}

func TestCheckoutTaskLinksExistingBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	initializeGitRepo(t, root)

	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	task, err := CreateTask(root, CreateTaskInput{Title: "Root Feature", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	repo, err := OpenGitRepo(root)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}
	if err := repo.CheckoutOrCreateBranch("feature/manual-link"); err != nil {
		t.Fatalf("CheckoutOrCreateBranch manual: %v", err)
	}
	if err := repo.CheckoutExistingBranch("main"); err != nil {
		t.Fatalf("CheckoutExistingBranch main: %v", err)
	}

	result, err := CheckoutTask(root, task.ID, CheckoutTaskInput{
		ExistingBranch: "feature/manual-link",
	})
	if err != nil {
		t.Fatalf("CheckoutTask existing branch: %v", err)
	}

	if result.Branch != "feature/manual-link" {
		t.Fatalf("expected linked branch feature/manual-link, got %s", result.Branch)
	}
}

func initializeGitRepo(t *testing.T, root string) {
	t.Helper()

	runGitCommand(t, root, "init", "-b", "main")
	runGitCommand(t, root, "config", "user.email", "tasker@example.com")
	runGitCommand(t, root, "config", "user.name", "Tasker Tests")

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile README: %v", err)
	}

	runGitCommand(t, root, "add", "README.md")
	runGitCommand(t, root, "commit", "-m", "init")
}

func runGitCommand(t *testing.T, root string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
