package tui

import (
	"strings"
	"testing"

	"github.com/bamorial/tasker/internal/tasker"
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
	if !strings.Contains(got.detailViewport.View(), "Task "+second.ID+": Beta") {
		t.Fatalf("expected detail viewport to include selected task, got %q", got.detailViewport.View())
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

func mustSnapshot(t *testing.T, root string) *tasker.WorkspaceSnapshot {
	t.Helper()

	snapshot, err := tasker.LoadWorkspaceSnapshot(root)
	if err != nil {
		t.Fatalf("LoadWorkspaceSnapshot: %v", err)
	}
	return snapshot
}
