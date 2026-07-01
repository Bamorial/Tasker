package tasker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCurrentWorkspaceSnapshotIgnoresMissingCurrentTask(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	contextPath := filepath.Join(root, TaskerDirName, "current", "CONTEXT.json")
	payload := map[string]any{
		"current_task_id": "046",
		"task_id":         "046",
		"current_task": map[string]any{
			"id": "046",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal context: %v", err)
	}
	if err := os.WriteFile(contextPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile context: %v", err)
	}

	snapshot, err := ReadCurrentWorkspaceSnapshot(root)
	if err != nil {
		t.Fatalf("ReadCurrentWorkspaceSnapshot: %v", err)
	}
	if snapshot.Task != nil {
		t.Fatalf("expected missing current task to be ignored, got %#v", snapshot.Task)
	}
	if got := snapshot.Context["current_task_id"]; got != "046" {
		t.Fatalf("expected stale context to remain available, got %#v", got)
	}
}
