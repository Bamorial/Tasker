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

func TestReadCodexSessionOutputDeduplicatesAdjacentAgentMessages(t *testing.T) {
	homeDir := t.TempDir()
	setEnvForTest(t, "HOME", homeDir)

	sessionID := "session-duplicate-123"
	sessionPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "01", "rollout-2026-07-01T10-10-00-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	content := strings.Join([]string{
		`{"timestamp":"2026-07-01T07:10:00Z","type":"session_meta","payload":{"session_id":"session-duplicate-123","id":"session-duplicate-123"}}`,
		`{"timestamp":"2026-07-01T07:10:01Z","type":"event_msg","payload":{"type":"agent_message","message":"Same line once"}}`,
		`{"timestamp":"2026-07-01T07:10:02Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Same line once"}]}}`,
		`{"timestamp":"2026-07-01T07:10:03Z","type":"event_msg","payload":{"type":"agent_message","message":"Another line"}}`,
		"",
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	output, err := ReadCodexSessionOutput(sessionID)
	if err != nil {
		t.Fatalf("ReadCodexSessionOutput: %v", err)
	}

	if output != "Same line once\n\nAnother line" {
		t.Fatalf("unexpected deduplicated output: %q", output)
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

func TestReadTaskAgentOutputFallsBackToTaskLiveOutput(t *testing.T) {
	unsetEnvForTest(t, "CODEX_THREAD_ID")
	unsetEnvForTest(t, "TASKER_SESSION_ID")
	unsetEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Live output", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if err := os.WriteFile(TaskLiveOutputPath(task), []byte("Streaming line 1\nStreaming line 2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile live output: %v", err)
	}

	session, output, err := ReadTaskAgentOutput(task)
	if err != nil {
		t.Fatalf("ReadTaskAgentOutput: %v", err)
	}
	if session.Source != "task live output" {
		t.Fatalf("expected live output source, got %#v", session)
	}
	if output != "Streaming line 1\nStreaming line 2" {
		t.Fatalf("unexpected live output: %q", output)
	}
}

func TestReadTaskAgentOutputUsesRunningExecutionSessionBeforeTaskSessionStored(t *testing.T) {
	homeDir := t.TempDir()
	setEnvForTest(t, "HOME", homeDir)
	unsetEnvForTest(t, "CODEX_THREAD_ID")
	unsetEnvForTest(t, "TASKER_SESSION_ID")
	unsetEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Running execution output", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	startedAt := time.Date(2026, 7, 1, 12, 2, 57, 0, time.FixedZone("EEST", 3*60*60))
	if err := UpdateTaskStatus(task, "RUNNING", "codex", startedAt); err != nil {
		t.Fatalf("UpdateTaskStatus: %v", err)
	}
	if err := WriteTaskExecutionState(task, TaskExecutionState{
		PID:       123,
		PGID:      123,
		StartedAt: startedAt.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("WriteTaskExecutionState: %v", err)
	}

	sessionID := "running-exec-session-123"
	sessionPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "07", "01", "rollout-2026-07-01T12-02-57-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(sessionPath, []byte(strings.Join([]string{
		`{"timestamp":"2026-07-01T09:02:57Z","type":"session_meta","payload":{"session_id":"running-exec-session-123","id":"running-exec-session-123","timestamp":"2026-07-01T09:02:57Z","cwd":"` + root + `","originator":"codex_exec","source":"exec"}}`,
		`{"timestamp":"2026-07-01T09:03:02Z","type":"event_msg","payload":{"type":"agent_message","message":"Running output from persisted exec session"}}`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile session: %v", err)
	}

	reloaded, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask reload: %v", err)
	}

	session, output, err := ReadTaskAgentOutput(reloaded)
	if err != nil {
		t.Fatalf("ReadTaskAgentOutput: %v", err)
	}
	if session.ID != sessionID {
		t.Fatalf("expected execution session %s, got %#v", sessionID, session)
	}
	if session.Source != "codex exec session file" {
		t.Fatalf("expected execution session source, got %#v", session)
	}
	if output != "Running output from persisted exec session" {
		t.Fatalf("unexpected execution output: %q", output)
	}
}
