package tasker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadCodexSessionOutputExtractsUserFacingMessages(t *testing.T) {
	homeDir := t.TempDir()
	setEnvForTest(t, "HOME", homeDir)

	sessionID := "session-output-123"
	sessionPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "01", "rollout-2026-07-01T10-00-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	content := strings.Join([]string{
		`{"timestamp":"2026-07-01T07:00:00Z","type":"session_meta","payload":{"session_id":"session-output-123","id":"session-output-123"}}`,
		`{"timestamp":"2026-07-01T07:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"Inspecting the workspace"}}`,
		`{"timestamp":"2026-07-01T07:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Implemented the fix."}]}}`,
		`{"timestamp":"2026-07-01T07:00:03Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"ignore me"}]}}`,
		"",
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	output, err := ReadCodexSessionOutput(sessionID)
	if err != nil {
		t.Fatalf("ReadCodexSessionOutput: %v", err)
	}

	if output != "Inspecting the workspace\n\nImplemented the fix." {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestReadTaskAgentOutputUsesLatestCodexSession(t *testing.T) {
	homeDir := t.TempDir()
	setEnvForTest(t, "HOME", homeDir)

	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Transcript", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if err := StoreTaskSession(task, newCodexTaskSession("old-session", "test", time.Date(2026, 7, 1, 7, 0, 0, 0, time.UTC))); err != nil {
		t.Fatalf("StoreTaskSession old: %v", err)
	}
	if err := StoreTaskSession(task, newCodexTaskSession("new-session", "test", time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC))); err != nil {
		t.Fatalf("StoreTaskSession new: %v", err)
	}

	sessionPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "01", "rollout-2026-07-01T11-00-00-new-session.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(sessionPath, []byte(strings.Join([]string{
		`{"timestamp":"2026-07-01T08:00:00Z","type":"session_meta","payload":{"session_id":"new-session","id":"new-session"}}`,
		`{"timestamp":"2026-07-01T08:00:01Z","type":"event_msg","payload":{"type":"agent_message","message":"Newest output"}}`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reloaded, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask reload: %v", err)
	}

	session, output, err := ReadTaskAgentOutput(reloaded)
	if err != nil {
		t.Fatalf("ReadTaskAgentOutput: %v", err)
	}
	if session.ID != "new-session" {
		t.Fatalf("expected latest session, got %#v", session)
	}
	if output != "Newest output" {
		t.Fatalf("unexpected task output: %q", output)
	}
}
