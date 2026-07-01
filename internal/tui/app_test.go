package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bamorial/tasker/internal/tasker"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRefreshLoadsWorkspaceSnapshotAndSelectsCurrentTask(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	first, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Alpha", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}
	second, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Beta", Type: "bug"})
	if err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}
	if _, err := tasker.CheckoutTask(root, second.ID, tasker.CheckoutTaskInput{NoBranch: true}); err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}

	m := newModel(root)
	updated, cmd := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	if cmd != nil {
		t.Fatalf("expected no follow-up command, got %#v", cmd)
	}

	got := updated.(model)
	if got.snapshot == nil {
		t.Fatal("expected snapshot to load")
	}
	if got.selectedTaskID != second.ID {
		t.Fatalf("expected current task %s to be selected, got %s", second.ID, got.selectedTaskID)
	}
	if len(got.filtered) != 2 {
		t.Fatalf("expected two tasks in filtered tree, got %d", len(got.filtered))
	}
	if !strings.Contains(got.currentViewport.View(), "Task: "+second.ID+" Beta") {
		t.Fatalf("expected current viewport to include selected task, got %q", got.currentViewport.View())
	}
	if got.snapshot.Current.Task == nil || got.snapshot.Current.Task.Meta.ID != second.ID {
		t.Fatalf("expected snapshot current task %s, got %#v", second.ID, got.snapshot.Current.Task)
	}
	if got.snapshot.StatusCounts["NEW"] != 2 {
		t.Fatalf("expected NEW count 2, got %#v", got.snapshot.StatusCounts)
	}
	if got.snapshot.Tasks[0].Meta.ID != first.ID {
		t.Fatalf("expected tasks sorted by id, got %#v", got.snapshot.Tasks)
	}
}

func TestSubmitNewTaskCreatesTaskWithoutOpeningEditor(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	m := newModel(root)
	msg := m.submitNewTask(map[string]string{
		"title":         "Ship TUI",
		"type":          "feature",
		"checkout_mode": "none",
		"open_behavior": "no-open",
		"open_target":   "task",
	})()

	result, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %#v", msg)
	}
	if result.Err != nil {
		t.Fatalf("submitNewTask error = %v", result.Err)
	}
	if !result.Refresh {
		t.Fatalf("expected refresh after creating task, got %#v", result)
	}
	if result.SelectedID == "" {
		t.Fatalf("expected created task id, got %#v", result)
	}

	task, err := tasker.GetTask(root, result.SelectedID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Meta.Title != "Ship TUI" {
		t.Fatalf("expected created title %q, got %#v", "Ship TUI", task.Meta)
	}
}

func TestNewTaskFormDefaultsToOpenWhenEditorConfigured(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, tasker.TaskerDirName, "config.yaml"), []byte("editor: \"nvim\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	m := newModel(root)
	form := m.newTaskForm()

	if got := form.Values()["open_behavior"]; got != "open" {
		t.Fatalf("expected open_behavior default %q, got %q", "open", got)
	}
}

func TestNewTaskFormDefaultsToNoOpenWithoutEditor(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	m := newModel(root)
	form := m.newTaskForm()

	if got := form.Values()["open_behavior"]; got != "no-open" {
		t.Fatalf("expected open_behavior default %q, got %q", "no-open", got)
	}
}

func TestTasksListContentShowsIDStatusThenTitle(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	task, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Color badge", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	statusBytes, err := json.Marshal(tasker.TaskStatus{
		Status:  "DONE",
		Agent:   "codex",
		Started: "2026-07-01T13:00:00+03:00",
	})
	if err != nil {
		t.Fatalf("Marshal status: %v", err)
	}
	if err := os.WriteFile(filepath.Join(task.Path, "status.json"), statusBytes, 0o644); err != nil {
		t.Fatalf("WriteFile status: %v", err)
	}

	m := newModel(root)
	m.width = 160
	m.height = 40
	m.taskViewport.Width = 120
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model).tasksListContent()

	if !strings.Contains(got, task.ID+" \x1b[") {
		t.Fatalf("expected task id before ANSI-colored status badge, got %q", got)
	}
	badgeIndex := strings.Index(got, "[DONE]")
	titleIndex := strings.Index(got, "Color badge")
	if badgeIndex == -1 || titleIndex == -1 || badgeIndex > titleIndex {
		t.Fatalf("expected task row to include status badge before title, got %q", got)
	}
}

func TestTasksListContentMarksCurrentTask(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	first, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Alpha", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}
	second, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Beta", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}
	if _, err := tasker.CheckoutTask(root, second.ID, tasker.CheckoutTaskInput{NoBranch: true}); err != nil {
		t.Fatalf("CheckoutTask: %v", err)
	}

	m := newModel(root)
	m.width = 160
	m.height = 40
	m.taskViewport.Width = 120
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model).tasksListContent()

	if !strings.Contains(got, "*") {
		t.Fatalf("expected current task marker, got %q", got)
	}
	if strings.Index(got, second.ID) < strings.Index(got, first.ID) {
		t.Fatalf("expected both task ids to remain visible, got %q", got)
	}
}

func TestWorkerEnterOpensAgentOutput(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)

	homeDir := t.TempDir()
	setTUIEnvForTest(t, "HOME", homeDir)

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Running output", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	session := tasker.TaskSession{
		Agent:         "codex",
		ID:            "worker-session-123",
		RecordedAt:    "2026-07-01T13:00:00+03:00",
		ResumeCommand: "codex resume worker-session-123",
		ForkCommand:   "codex fork worker-session-123",
	}
	if err := tasker.StoreTaskSession(task, session); err != nil {
		t.Fatalf("StoreTaskSession: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "RUNNING", "codex", time.Date(2026, 7, 1, 13, 0, 0, 0, time.FixedZone("EEST", 3*60*60))); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	sessionPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "01", "rollout-2026-07-01T10-00-00-worker-session-123.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(sessionPath, []byte(strings.Join([]string{
		`{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"session_id":"worker-session-123","id":"worker-session-123"}}`,
		`{"timestamp":"2026-07-01T10:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"Streaming progress"}}`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile session: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)
	got.focus = panelWorkers

	opened, _ := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := *(opened.(*model))

	if after.focus != panelCurrent {
		t.Fatalf("expected focus to move to current panel, got %v", after.focus)
	}
	if after.currentViewMode != viewAgent {
		t.Fatalf("expected agent view, got %s", after.currentViewMode)
	}
	if !strings.Contains(after.currentViewport.View(), "Streaming progress") {
		t.Fatalf("expected current viewport to include agent output, got %q", after.currentViewport.View())
	}
}

func setTUIEnvForTest(t *testing.T, name, value string) {
	t.Helper()

	oldValue, hadValue := os.LookupEnv(name)
	if err := os.Setenv(name, value); err != nil {
		t.Fatalf("Setenv %s: %v", name, err)
	}

	t.Cleanup(func() {
		var err error
		if hadValue {
			err = os.Setenv(name, oldValue)
		} else {
			err = os.Unsetenv(name)
		}
		if err != nil {
			t.Fatalf("restore env %s: %v", name, err)
		}
	})
}

func mustSnapshot(t *testing.T, root string) *tasker.WorkspaceSnapshot {
	t.Helper()

	snapshot, err := tasker.LoadWorkspaceSnapshot(root)
	if err != nil {
		t.Fatalf("LoadWorkspaceSnapshot: %v", err)
	}
	return snapshot
}
