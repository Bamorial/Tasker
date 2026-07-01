package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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

func TestWorkerEnterOpensLiveTaskOutputBeforeSessionTranscriptExists(t *testing.T) {
	unsetTUIEnvForTest(t, "CODEX_THREAD_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetTUIEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetTUIEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Live running output", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "RUNNING", "codex", time.Date(2026, 7, 1, 13, 0, 0, 0, time.FixedZone("EEST", 3*60*60))); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := os.WriteFile(tasker.TaskLiveOutputPath(task), []byte("Live transcript line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile live output: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)
	got.focus = panelWorkers

	opened, _ := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := *(opened.(*model))

	if !strings.Contains(after.currentViewport.View(), "Live transcript line") {
		t.Fatalf("expected current viewport to include live task output, got %q", after.currentViewport.View())
	}
	if !strings.Contains(after.currentViewport.View(), "Source: task live output") {
		t.Fatalf("expected current viewport to identify live task output source, got %q", after.currentViewport.View())
	}
}

func TestTasksEnterOpensRunningTaskInAgentView(t *testing.T) {
	unsetTUIEnvForTest(t, "CODEX_THREAD_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetTUIEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetTUIEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Running current view", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "RUNNING", "codex", time.Date(2026, 7, 1, 13, 0, 0, 0, time.FixedZone("EEST", 3*60*60))); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := os.WriteFile(tasker.TaskLiveOutputPath(task), []byte("Current panel live output\n"), 0o644); err != nil {
		t.Fatalf("WriteFile live output: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)

	opened, _ := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := *(opened.(*model))

	if after.focus != panelCurrent {
		t.Fatalf("expected focus to move to current panel, got %v", after.focus)
	}
	if after.currentViewMode != viewAgent {
		t.Fatalf("expected running task to open in agent view, got %s", after.currentViewMode)
	}
	if !strings.Contains(after.currentViewport.View(), "Current panel live output") {
		t.Fatalf("expected current viewport to include running task output, got %q", after.currentViewport.View())
	}
}

func TestTasksOOpensSelectedTaskInAgentView(t *testing.T) {
	unsetTUIEnvForTest(t, "CODEX_THREAD_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetTUIEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetTUIEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Open agent view", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "RUNNING", "codex", time.Date(2026, 7, 1, 13, 0, 0, 0, time.FixedZone("EEST", 3*60*60))); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := os.WriteFile(tasker.TaskLiveOutputPath(task), []byte("Open with o\n"), 0o644); err != nil {
		t.Fatalf("WriteFile live output: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)
	got.focus = panelTasks

	opened, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	after := *(opened.(*model))

	if after.focus != panelCurrent {
		t.Fatalf("expected focus to move to current panel, got %v", after.focus)
	}
	if after.currentViewMode != viewAgent {
		t.Fatalf("expected agent view, got %s", after.currentViewMode)
	}
	if !strings.Contains(after.currentViewport.View(), "Open with o") {
		t.Fatalf("expected current viewport to include agent output, got %q", after.currentViewport.View())
	}
}

func TestCustomTaskKeybindingOpensCurrentPanel(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, tasker.TaskerDirName, "config.yaml"), []byte(`editor: ""
tui:
  keybindings:
    global:
      toggle_help: ["H"]
    tasks:
      open_current: ["l"]
`), 0o644); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}

	if _, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Custom open binding", Type: "feature"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)

	opened, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	after := *(opened.(*model))

	if after.focus != panelCurrent {
		t.Fatalf("expected focus to move to current panel, got %v", after.focus)
	}
	if after.currentViewMode != viewAuto {
		t.Fatalf("expected current view mode %s, got %s", viewAuto, after.currentViewMode)
	}
	if !strings.Contains(after.renderHelp(), "H toggles help") {
		t.Fatalf("expected help to show custom help key, got %q", after.renderHelp())
	}
}

func TestCurrentViewOShowsAgentOutput(t *testing.T) {
	unsetTUIEnvForTest(t, "CODEX_THREAD_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetTUIEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetTUIEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Current output shortcut", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "RUNNING", "codex", time.Date(2026, 7, 1, 13, 0, 0, 0, time.FixedZone("EEST", 3*60*60))); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := os.WriteFile(tasker.TaskLiveOutputPath(task), []byte("Current panel shortcut\n"), 0o644); err != nil {
		t.Fatalf("WriteFile live output: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	m.currentViewMode = viewTask
	m.focus = panelCurrent
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)
	got.focus = panelCurrent
	got.currentViewMode = viewTask
	got.syncDerivedState()

	opened, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	after := *(opened.(*model))

	if after.currentViewMode != viewAgent {
		t.Fatalf("expected agent view, got %s", after.currentViewMode)
	}
	if after.focus != panelCurrent {
		t.Fatalf("expected focus to stay on current panel, got %v", after.focus)
	}
	if !strings.Contains(after.currentViewport.View(), "Current panel shortcut") {
		t.Fatalf("expected current viewport to include agent output, got %q", after.currentViewport.View())
	}
}

func TestCurrentViewScrollPersistsAcrossSnapshotRefresh(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Scrollable task", Type: "bug"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	lines := []string{"# Task", "", "Scroll regression"}
	for i := 0; i < 40; i++ {
		lines = append(lines, "line "+strings.Repeat("x", 24))
	}
	if err := os.WriteFile(task.TaskFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}

	m := newModel(root)
	m.width = 100
	m.height = 18
	m.focus = panelCurrent
	m.currentViewMode = viewTask
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)

	scrolled, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	scrolledModel := *(scrolled.(*model))
	if scrolledModel.currentViewport.YOffset == 0 {
		t.Fatal("expected current viewport to scroll down")
	}

	refreshed, _ := scrolledModel.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	after := refreshed.(model)

	if after.currentViewport.YOffset != scrolledModel.currentViewport.YOffset {
		t.Fatalf("expected Y offset %d after refresh, got %d", scrolledModel.currentViewport.YOffset, after.currentViewport.YOffset)
	}
	if !strings.Contains(after.currentViewport.View(), "Scroll regression") {
		t.Fatalf("expected current viewport content to remain loaded, got %q", after.currentViewport.View())
	}
}

func TestFileChangeMessageRefreshesWorkspaceSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Refresh me", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)

	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "DONE", "codex", time.Date(2026, 7, 1, 13, 0, 0, 0, time.FixedZone("EEST", 3*60*60))); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	nextModel, cmd := got.Update(fileChangeMsg{})
	if cmd == nil {
		t.Fatal("expected refresh command after file change")
	}

	refreshMsg := cmd()
	snapshot, ok := refreshMsg.(snapshotMsg)
	if !ok {
		t.Fatalf("expected snapshotMsg, got %#v", refreshMsg)
	}

	afterRefresh, _ := nextModel.(model).Update(snapshot)
	final := afterRefresh.(model)

	if final.snapshot == nil {
		t.Fatal("expected snapshot after refresh")
	}
	if final.snapshot.Tasks[0].Status.Status != "DONE" {
		t.Fatalf("expected refreshed task status DONE, got %#v", final.snapshot.Tasks[0].Status)
	}
	if !strings.Contains(final.tasksListContent(), "[DONE]") {
		t.Fatalf("expected task list to include updated DONE badge, got %q", final.tasksListContent())
	}
}

func TestWorkerStopConfirmationCancelsRunningTask(t *testing.T) {
	unsetTUIEnvForTest(t, "CODEX_THREAD_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_ID")
	unsetTUIEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetTUIEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetTUIEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Cancelable", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := tasker.UpdateTaskStatus(task, "RUNNING", "codex", time.Now()); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}

	cmd := exec.Command("sh", "-c", "trap 'exit 0' TERM; while true; do sleep 1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start helper process: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	if err := tasker.WriteTaskExecutionState(task, tasker.TaskExecutionState{
		PID:       cmd.Process.Pid,
		PGID:      cmd.Process.Pid,
		StartedAt: time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("WriteTaskExecutionState: %v", err)
	}

	m := newModel(root)
	m.width = 120
	m.height = 40
	updated, _ := m.Update(snapshotMsg{Snapshot: mustSnapshot(t, root)})
	got := updated.(model)
	got.focus = panelWorkers
	got.selectedWorkerTaskID = created.ID

	withConfirm, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	confirmModel := *(withConfirm.(*model))
	if confirmModel.confirm == nil || confirmModel.confirm.Kind != modalStopExecution {
		t.Fatalf("expected stop confirmation modal, got %#v", confirmModel.confirm)
	}

	toggled, _ := confirmModel.Update(tea.KeyMsg{Type: tea.KeyRight})
	confirmed, cmdMsg := toggled.(*model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmdMsg == nil {
		t.Fatal("expected stop confirmation to return a command")
	}
	msg := cmdMsg()
	result, ok := msg.(mutationResultMsg)
	if !ok {
		t.Fatalf("expected mutationResultMsg, got %#v", msg)
	}
	if result.Err != nil {
		t.Fatalf("stop result error = %v", result.Err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("expected helper process to exit after worker stop")
	}

	reloaded, err := tasker.GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask reload: %v", err)
	}
	if reloaded.Status.Status != "CANCELLED" {
		t.Fatalf("expected CANCELLED status, got %#v", reloaded.Status)
	}

	finalModel := *(confirmed.(*model))
	if finalModel.confirm != nil {
		t.Fatalf("expected confirmation modal to close, got %#v", finalModel.confirm)
	}
}

func TestListTaskerWatchDirsIncludesNestedTaskDirectories(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}
	child, err := tasker.CreateTask(root, tasker.CreateTaskInput{Title: "Child", Type: "feature", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}

	dirs, err := listTaskerWatchDirs(root)
	if err != nil {
		t.Fatalf("listTaskerWatchDirs: %v", err)
	}

	want := []string{
		filepath.Join(root, tasker.TaskerDirName),
		filepath.Join(parent.Path, "sessions"),
		filepath.Join(parent.Path, "children"),
		child.Path,
		filepath.Join(child.Path, "sessions"),
	}
	for _, dir := range want {
		if !containsString(dirs, dir) {
			t.Fatalf("expected watch dirs to include %s, got %#v", dir, dirs)
		}
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

func unsetTUIEnvForTest(t *testing.T, name string) {
	t.Helper()

	oldValue, hadValue := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv %s: %v", name, err)
	}

	t.Cleanup(func() {
		if !hadValue {
			return
		}
		if err := os.Setenv(name, oldValue); err != nil {
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
