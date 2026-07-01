package tasker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setEnvForTest(t *testing.T, name, value string) {
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

func unsetEnvForTest(t *testing.T, name string) {
	t.Helper()

	oldValue, hadValue := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv %s: %v", name, err)
	}

	t.Cleanup(func() {
		var err error
		if hadValue {
			err = os.Setenv(name, oldValue)
		}
		if err != nil {
			t.Fatalf("restore env %s: %v", name, err)
		}
	})
}

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

func TestGetTaskIgnoresUnrelatedInvalidTaskStatus(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	first, err := CreateTask(root, CreateTaskInput{Title: "Legacy", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask first: %v", err)
	}
	second, err := CreateTask(root, CreateTaskInput{Title: "Target", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask second: %v", err)
	}

	if err := writeJSON(filepath.Join(first.Path, "status.json"), TaskStatus{
		Status:  "completed",
		Agent:   "worker",
		Started: "2026-06-30T10:00:00+03:00",
	}); err != nil {
		t.Fatalf("write legacy status: %v", err)
	}

	task, err := GetTask(root, second.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.Meta.ID != second.ID {
		t.Fatalf("expected task %s, got %s", second.ID, task.Meta.ID)
	}
}

func TestInitializeWorkspaceCreatesTaskTemplates(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	templates := []string{
		"default.md",
		"bug.md",
		"decision.md",
		"documentation.md",
		"feature.md",
		"research.md",
		"review.md",
		"test.md",
	}

	for _, name := range templates {
		path := filepath.Join(root, TaskerDirName, "templates", "tasks", name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected template %s to exist: %v", path, err)
		}
	}

	importTemplatePath := filepath.Join(root, TaskerDirName, "templates", "import-tasks.json")
	if _, err := os.Stat(importTemplatePath); err != nil {
		t.Fatalf("expected import template %s to exist: %v", importTemplatePath, err)
	}

	importsDir := filepath.Join(root, TaskerDirName, "imports")
	if _, err := os.Stat(importsDir); err != nil {
		t.Fatalf("expected imports dir %s to exist: %v", importsDir, err)
	}
}

func TestInitializeWorkspaceWritesAgentTaskCreationRule(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	for _, path := range []string{
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, TaskerDirName, "START.md"),
		filepath.Join(root, TaskerDirName, "agent.md"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}

		content := string(data)
		for _, want := range []string{
			"tasker new",
			"tasker add",
			"tasker import",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("expected %s to include %q, got:\n%s", path, want, content)
			}
		}
		if !strings.Contains(strings.ToLower(content), "do not manually create task folders") &&
			!strings.Contains(strings.ToLower(content), "never create task folders") {
			t.Fatalf("expected %s to include a manual task creation prohibition, got:\n%s", path, content)
		}
	}
}

func TestCreateTaskUsesFeatureTemplateWhenTypeOmitted(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Untyped Task"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	data, err := os.ReadFile(created.TaskFile)
	if err != nil {
		t.Fatalf("ReadFile task.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{
		"# Feature",
		"ID: " + created.ID,
		"Type: feature",
		"## Goal",
		"## Details",
		"## Acceptance",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected implicit feature template to include %q, got:\n%s", want, content)
		}
	}
}

func TestCreateTaskUsesTaskTypeTemplate(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Fix Login", Type: "bug"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	data, err := os.ReadFile(created.TaskFile)
	if err != nil {
		t.Fatalf("ReadFile task.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{
		"# Bug",
		"## Problem",
		"## Steps",
		"## Expected",
		"Type: bug",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected bug template to include %q, got:\n%s", want, content)
		}
	}

	if strings.Contains(content, "[write here]") {
		t.Fatalf("expected bug template to avoid legacy placeholder text, got:\n%s", content)
	}
}

func TestCreateTaskUsesCustomizedWorkspaceTemplate(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	customTemplate := "# Custom Feature\n\nTask {{ID}}: {{TITLE}}\n\n## Ship\n\n[write here]\n"
	templatePath := filepath.Join(root, TaskerDirName, "templates", "tasks", "feature.md")
	if err := os.WriteFile(templatePath, []byte(customTemplate), 0o644); err != nil {
		t.Fatalf("WriteFile custom template: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Launch API", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	data, err := os.ReadFile(created.TaskFile)
	if err != nil {
		t.Fatalf("ReadFile task.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{
		"# Custom Feature",
		"Task " + created.ID + ": Launch API",
		"## Ship",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected customized template to include %q, got:\n%s", want, content)
		}
	}
}

func TestCreateTaskUsesCustomizedFeatureTemplateWhenTypeOmitted(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	customTemplate := "# Workspace Feature Default\n\nShip {{TITLE}} as {{ID}}.\n"
	templatePath := filepath.Join(root, TaskerDirName, "templates", "tasks", "feature.md")
	if err := os.WriteFile(templatePath, []byte(customTemplate), 0o644); err != nil {
		t.Fatalf("WriteFile custom template: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Launch API"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	data, err := os.ReadFile(created.TaskFile)
	if err != nil {
		t.Fatalf("ReadFile task.md: %v", err)
	}

	content := string(data)
	for _, want := range []string{
		"# Workspace Feature Default",
		"Ship Launch API as " + created.ID + ".",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected implicit feature template to use customized workspace template and include %q, got:\n%s", want, content)
		}
	}
}

func TestCreateTaskStoresDetectedCodexSession(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	sessionID := "019f18b6-d117-7de2-ae23-4aa2ffaff8a1"
	setEnvForTest(t, "CODEX_THREAD_ID", sessionID)

	created, err := CreateTask(root, CreateTaskInput{Title: "Track session", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if len(task.Status.Sessions) != 1 {
		t.Fatalf("expected 1 stored session, got %#v", task.Status.Sessions)
	}

	session := task.Status.Sessions[0]
	if session.Agent != "codex" || session.ID != sessionID {
		t.Fatalf("unexpected stored session: %#v", session)
	}
	if session.ResumeCommand != "codex resume "+sessionID {
		t.Fatalf("expected resume command to be populated, got %#v", session)
	}
	if session.ForkCommand != "codex fork "+sessionID {
		t.Fatalf("expected fork command to be populated, got %#v", session)
	}

	var index TaskSessionIndex
	if err := readJSON(filepath.Join(created.Path, "sessions", "index.json"), &index); err != nil {
		t.Fatalf("readJSON session index: %v", err)
	}
	if len(index.Sessions) != 1 || index.Sessions[0].ID != sessionID {
		t.Fatalf("expected session index to mirror stored session, got %#v", index)
	}
}

func TestCreateTaskStoresExplicitSessionMetadata(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	unsetEnvForTest(t, "CODEX_THREAD_ID")
	setEnvForTest(t, "TASKER_SESSION_ID", "session-123")
	setEnvForTest(t, "TASKER_SESSION_AGENT", "claude")
	setEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND", "claude resume session-123")
	setEnvForTest(t, "TASKER_SESSION_FORK_COMMAND", "claude fork session-123")

	created, err := CreateTask(root, CreateTaskInput{Title: "Track explicit session", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if len(task.Status.Sessions) != 1 {
		t.Fatalf("expected 1 stored session, got %#v", task.Status.Sessions)
	}

	session := task.Status.Sessions[0]
	if session.Agent != "claude" || session.ID != "session-123" {
		t.Fatalf("unexpected stored session: %#v", session)
	}
	if session.ResumeCommand != "claude resume session-123" || session.ForkCommand != "claude fork session-123" {
		t.Fatalf("expected explicit commands to be preserved, got %#v", session)
	}
}

func TestStoreTaskSessionPersistsStatusAndIndex(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	unsetEnvForTest(t, "CODEX_THREAD_ID")
	unsetEnvForTest(t, "TASKER_SESSION_ID")
	unsetEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	created, err := CreateTask(root, CreateTaskInput{Title: "Store session", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	session := newCodexTaskSession("session-123", "codex exec", time.Date(2026, 6, 30, 14, 0, 0, 0, time.UTC))
	if err := StoreTaskSession(task, session); err != nil {
		t.Fatalf("StoreTaskSession: %v", err)
	}

	reloaded, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask reload: %v", err)
	}
	if len(reloaded.Status.Sessions) != 1 {
		t.Fatalf("expected one stored session, got %#v", reloaded.Status.Sessions)
	}
	if reloaded.Status.Sessions[0].ID != "session-123" {
		t.Fatalf("expected persisted stored session, got %#v", reloaded.Status.Sessions)
	}

	var index TaskSessionIndex
	if err := readJSON(filepath.Join(created.Path, "sessions", "index.json"), &index); err != nil {
		t.Fatalf("readJSON index: %v", err)
	}
	if len(index.Sessions) != 1 || index.Sessions[0].ID != "session-123" {
		t.Fatalf("expected session index to contain stored session, got %#v", index)
	}
}

func TestDoTaskStoresHeadlessCodexSession(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	unsetEnvForTest(t, "CODEX_THREAD_ID")
	unsetEnvForTest(t, "TASKER_SESSION_ID")
	unsetEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	created, err := CreateTask(root, CreateTaskInput{Title: "Headless", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	originalExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=TestHelperProcessHeadlessCodex", "--", name}, args...)
		cmd := exec.Command(os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		return cmd
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	var stdout strings.Builder
	var stderr strings.Builder
	if err := DoTask(root, created.ID, &stdout, &stderr); err != nil {
		t.Fatalf("DoTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if len(task.Status.Sessions) != 1 {
		t.Fatalf("expected one stored session, got %#v", task.Status.Sessions)
	}
	if task.Status.Status != "DONE" {
		t.Fatalf("expected successful headless run to finish as DONE when no final status is written, got %#v", task.Status)
	}
	if task.Status.Sessions[0].ID != "session-headless-123" {
		t.Fatalf("expected stored headless session ID, got %#v", task.Status.Sessions[0])
	}
	if !strings.Contains(stdout.String(), "Started Codex session: session-headless-123") {
		t.Fatalf("expected command output to include session id, got %q", stdout.String())
	}
}

func TestDoTaskSuppressesStdinNoticeWithoutLoader(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	unsetEnvForTest(t, "CODEX_THREAD_ID")
	unsetEnvForTest(t, "TASKER_SESSION_ID")
	unsetEnvForTest(t, "TASKER_SESSION_AGENT")
	unsetEnvForTest(t, "TASKER_SESSION_RESUME_COMMAND")
	unsetEnvForTest(t, "TASKER_SESSION_FORK_COMMAND")

	created, err := CreateTask(root, CreateTaskInput{Title: "Progress", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	originalExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=TestHelperProcessHeadlessCodexWithStderrNotice", "--", name}, args...)
		cmd := exec.Command(os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_STDERR_NOTICE=1")
		return cmd
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	var stdout strings.Builder
	var stderr strings.Builder
	if err := DoTask(root, created.ID, &stdout, &stderr); err != nil {
		t.Fatalf("DoTask: %v", err)
	}

	if strings.Contains(stdout.String(), "Tasker do is working.") {
		t.Fatalf("expected loader output to be removed, got %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "Reading additional input from stdin") {
		t.Fatalf("expected stdin notice to be suppressed, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Started Codex session: session-headless-789") {
		t.Fatalf("expected stored session output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Finished with loader") {
		t.Fatalf("expected codex output to still be shown, got %q", stdout.String())
	}
}

func TestHelperProcessHeadlessCodex(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprintln(os.Stdout, `{"type":"session_meta","payload":{"session_id":"session-headless-123","id":"session-headless-123"}}`)
	fmt.Fprintln(os.Stdout, `{"type":"event_msg","payload":{"type":"agent_message","message":"Working headlessly"}}`)
	fmt.Fprintln(os.Stdout, `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Finished task"}]}}`)
	os.Exit(0)
}

func TestHelperProcessHeadlessCodexWithStderrNotice(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_STDERR_NOTICE") != "1" {
		return
	}

	fmt.Fprintln(os.Stderr, "Reading additional input from stdin...")
	time.Sleep(20 * time.Millisecond)
	fmt.Fprintln(os.Stdout, `{"type":"session_meta","payload":{"session_id":"session-headless-789","id":"session-headless-789"}}`)
	fmt.Fprintln(os.Stdout, `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Finished with loader"}]}}`)
	os.Exit(0)
}

func TestHelperProcessHeadlessCodexWithoutSessionMeta(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_NO_SESSION_META") != "1" {
		return
	}

	fmt.Fprintln(os.Stdout, `{"type":"event_msg","payload":{"type":"agent_message","message":"Working headlessly"}}`)
	fmt.Fprintln(os.Stdout, `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Finished task"}]}}`)
	os.Exit(0)
}

func TestHelperProcessDetachedDoLaunch(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS_DETACHED_DO") != "1" {
		return
	}

	markerPath := os.Getenv("TASKER_DETACHED_MARKER")
	if markerPath == "" {
		os.Exit(2)
	}
	if err := os.WriteFile(markerPath, []byte(strings.Join(os.Args, "\n")), 0o644); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestDoTaskFallsBackToPersistedCodexExecSession(t *testing.T) {
	root := t.TempDir()
	unsetEnvForTest(t, "CODEX_THREAD_ID")
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Headless persisted session", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll home: %v", err)
	}
	setEnvForTest(t, "HOME", homeDir)

	sessionID := "session-from-file-456"
	sessionPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "06", "30", "rollout-2026-06-30T20-40-29-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("MkdirAll session dir: %v", err)
	}
	sessionMeta := fmt.Sprintf(
		"{\"timestamp\":\"2026-06-30T17:40:29.776Z\",\"type\":\"session_meta\",\"payload\":{\"session_id\":\"%s\",\"id\":\"%s\",\"timestamp\":\"9999-06-30T17:40:29Z\",\"cwd\":\"%s\",\"originator\":\"codex_exec\",\"source\":\"exec\"}}\n",
		sessionID,
		sessionID,
		root,
	)
	if err := os.WriteFile(sessionPath, []byte(sessionMeta), 0o644); err != nil {
		t.Fatalf("WriteFile session meta: %v", err)
	}

	originalExecCommand := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		cs := append([]string{"-test.run=TestHelperProcessHeadlessCodexWithoutSessionMeta", "--"}, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS_NO_SESSION_META=1")
		return cmd
	}
	t.Cleanup(func() {
		execCommand = originalExecCommand
	})

	var stdout strings.Builder
	var stderr strings.Builder
	if err := DoTask(root, created.ID, &stdout, &stderr); err != nil {
		t.Fatalf("DoTask: %v", err)
	}

	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if len(task.Status.Sessions) != 1 {
		t.Fatalf("expected one stored session, got %#v", task.Status.Sessions)
	}
	if task.Status.Sessions[0].ID != sessionID {
		t.Fatalf("expected stored persisted session ID, got %#v", task.Status.Sessions[0])
	}
	if !strings.Contains(stdout.String(), "Started Codex session: "+sessionID) {
		t.Fatalf("expected command output to include fallback session id, got %q", stdout.String())
	}
}

func TestStartDetachedDoTaskLaunchesCurrentExecutable(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Detached", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	originalExecutablePath := currentExecutablePath
	originalDetachedExecCommand := detachedExecCommand
	currentExecutablePath = func() (string, error) {
		return os.Args[0], nil
	}
	detachedExecCommand = func(name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=TestHelperProcessDetachedDoLaunch", "--", name}, args...)
		cmd := exec.Command(os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS_DETACHED_DO=1",
			"TASKER_DETACHED_MARKER="+filepath.Join(root, "detached.marker"),
		)
		return cmd
	}
	t.Cleanup(func() {
		currentExecutablePath = originalExecutablePath
		detachedExecCommand = originalDetachedExecCommand
	})

	if err := StartDetachedDoTask(root, created.ID); err != nil {
		t.Fatalf("StartDetachedDoTask: %v", err)
	}

	markerPath := filepath.Join(root, "detached.marker")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(markerPath); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("expected detached helper to write %s", markerPath)
}

func TestSessionsForActionFiltersByAvailableCommand(t *testing.T) {
	task := &Task{
		Meta: TaskMeta{ID: "018"},
		Status: TaskStatus{
			Sessions: []TaskSession{
				{Agent: "codex", ID: "resume-only", ResumeCommand: "codex resume resume-only"},
				{Agent: "claude", ID: "fork-only", ForkCommand: "claude fork fork-only"},
				{Agent: "noop", ID: "none"},
			},
		},
	}

	resumeSessions := SessionsForAction(task, AgentSessionResume)
	if len(resumeSessions) != 1 || resumeSessions[0].ID != "resume-only" {
		t.Fatalf("expected only resume-capable session, got %#v", resumeSessions)
	}

	forkSessions := SessionsForAction(task, AgentSessionFork)
	if len(forkSessions) != 1 || forkSessions[0].ID != "fork-only" {
		t.Fatalf("expected only fork-capable session, got %#v", forkSessions)
	}
}

func TestSelectTaskSessionReturnsSingleMatchWithoutPrompt(t *testing.T) {
	task := &Task{
		Meta: TaskMeta{ID: "018"},
		Status: TaskStatus{
			Sessions: []TaskSession{
				{Agent: "codex", ID: "one", ResumeCommand: "codex resume one"},
			},
		},
	}

	session, err := SelectTaskSession(task, AgentSessionResume, nil, nil)
	if err != nil {
		t.Fatalf("SelectTaskSession: %v", err)
	}
	if session.ID != "one" {
		t.Fatalf("expected session one, got %#v", session)
	}
}

func TestSelectTaskSessionPromptsForMultipleMatches(t *testing.T) {
	task := &Task{
		Meta: TaskMeta{ID: "018"},
		Status: TaskStatus{
			Sessions: []TaskSession{
				{Agent: "codex", ID: "one", ResumeCommand: "codex resume one"},
				{Agent: "claude", ID: "two", ResumeCommand: "claude resume two"},
			},
		},
	}

	in, err := os.CreateTemp(t.TempDir(), "session-choice-in")
	if err != nil {
		t.Fatalf("CreateTemp input: %v", err)
	}
	if _, err := in.WriteString("2\n"); err != nil {
		t.Fatalf("WriteString input: %v", err)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatalf("Seek input: %v", err)
	}

	out, err := os.CreateTemp(t.TempDir(), "session-choice-out")
	if err != nil {
		t.Fatalf("CreateTemp output: %v", err)
	}

	session, err := SelectTaskSession(task, AgentSessionResume, in, out)
	if err != nil {
		t.Fatalf("SelectTaskSession: %v", err)
	}
	if session.ID != "two" {
		t.Fatalf("expected session two, got %#v", session)
	}
}

func TestSelectTaskSessionErrorsWithoutPromptIO(t *testing.T) {
	task := &Task{
		Meta: TaskMeta{ID: "018"},
		Status: TaskStatus{
			Sessions: []TaskSession{
				{Agent: "codex", ID: "one", ResumeCommand: "codex resume one"},
				{Agent: "claude", ID: "two", ResumeCommand: "claude resume two"},
			},
		},
	}

	if _, err := SelectTaskSession(task, AgentSessionResume, nil, nil); err == nil {
		t.Fatal("expected multiple-session selection without IO to fail")
	}
}

func TestParseTaskImportDocument(t *testing.T) {
	doc, err := ParseTaskImportDocument([]byte(`{
  "tasks": [
    {
      "title": "Add import command",
      "type": "feature",
      "body": "# Feature\n\n## Goal\n\nShip imports.\n",
      "subtasks": [
        {
          "title": "Add recursive import",
          "type": "feature",
          "body": "# Feature\n"
        }
      ]
    }
  ]
}`))
	if err != nil {
		t.Fatalf("ParseTaskImportDocument: %v", err)
	}

	if len(doc.Tasks) != 1 {
		t.Fatalf("expected 1 root task, got %d", len(doc.Tasks))
	}
	spec := doc.Tasks[0]
	if spec.Title != "Add import command" {
		t.Fatalf("expected title to be parsed, got %q", spec.Title)
	}
	if spec.Type != "feature" {
		t.Fatalf("expected type to be parsed, got %q", spec.Type)
	}
	if !strings.Contains(spec.Body, "## Goal") {
		t.Fatalf("expected body to be preserved, got:\n%s", spec.Body)
	}
	if len(spec.Subtasks) != 1 || spec.Subtasks[0].Title != "Add recursive import" {
		t.Fatalf("expected subtasks to be parsed, got %#v", spec.Subtasks)
	}
}

func TestImportTasksCreatesTasksWithImportedBody(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	importPath := filepath.Join(root, "import-tasks.json")
	importDoc := `{
  "tasks": [
    {
      "title": "Imported task",
      "type": "documentation",
      "body": "# Documentation\n\n## Topic\n\nImported from a file.\n",
      "instructions": "# Task Instructions\n\nImported instructions.\n",
      "context": {
        "source": "json"
      }
    }
  ]
}`
	if err := os.WriteFile(importPath, []byte(importDoc), 0o644); err != nil {
		t.Fatalf("WriteFile import doc: %v", err)
	}

	result, err := ImportTasks(root, importPath, ImportTaskInput{})
	if err != nil {
		t.Fatalf("ImportTasks: %v", err)
	}
	if len(result.Created) != 1 {
		t.Fatalf("expected 1 created task, got %d", len(result.Created))
	}

	task, err := GetTask(root, result.Primary.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if task.Meta.Title != "Imported task" {
		t.Fatalf("expected imported title, got %q", task.Meta.Title)
	}
	if task.Meta.Type != "documentation" {
		t.Fatalf("expected imported type, got %q", task.Meta.Type)
	}

	data, err := os.ReadFile(task.TaskFile)
	if err != nil {
		t.Fatalf("ReadFile task.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# Documentation") || !strings.Contains(content, "Imported from a file.") {
		t.Fatalf("expected imported body to overwrite task.md, got:\n%s", content)
	}

	instructionsData, err := os.ReadFile(task.InstructionsFile)
	if err != nil {
		t.Fatalf("ReadFile instructions.md: %v", err)
	}
	if !strings.Contains(string(instructionsData), "Imported instructions.") {
		t.Fatalf("expected imported instructions to overwrite instructions.md, got:\n%s", string(instructionsData))
	}

	var context map[string]string
	if err := readJSON(filepath.Join(task.Path, "context.json"), &context); err != nil {
		t.Fatalf("readJSON context.json: %v", err)
	}
	if context["source"] != "json" {
		t.Fatalf("expected imported context to be written, got %#v", context)
	}
}

func TestImportTasksCreatesNestedSubtasks(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	importPath := filepath.Join(root, "nested-imports.json")
	importDoc := `{
  "tasks": [
    {
      "title": "Parent task",
      "type": "feature",
      "body": "# Feature\n",
      "subtasks": [
        {
          "title": "Child task",
          "type": "bug",
          "body": "# Bug\n",
          "subtasks": [
            {
              "title": "Grandchild task",
              "type": "review",
              "body": "# Review\n"
            }
          ]
        }
      ]
    }
  ]
}`
	if err := os.WriteFile(importPath, []byte(importDoc), 0o644); err != nil {
		t.Fatalf("WriteFile import doc: %v", err)
	}

	result, err := ImportTasks(root, importPath, ImportTaskInput{})
	if err != nil {
		t.Fatalf("ImportTasks: %v", err)
	}

	if len(result.Created) != 3 {
		t.Fatalf("expected 3 created tasks, got %d", len(result.Created))
	}

	parent, err := GetTask(root, result.Primary.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	children, err := childTasks(parent.Path)
	if err != nil {
		t.Fatalf("childTasks parent: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child task, got %d", len(children))
	}

	child := children[0]
	if child.Meta.Title != "Child task" || child.Meta.ParentID != parent.Meta.ID {
		t.Fatalf("expected imported child to be nested under parent, got %+v", child.Meta)
	}

	grandchildren, err := childTasks(child.Path)
	if err != nil {
		t.Fatalf("childTasks child: %v", err)
	}
	if len(grandchildren) != 1 {
		t.Fatalf("expected 1 grandchild task, got %d", len(grandchildren))
	}
	if grandchildren[0].Meta.Title != "Grandchild task" || grandchildren[0].Meta.ParentID != child.Meta.ID {
		t.Fatalf("expected imported grandchild to be nested under child, got %+v", grandchildren[0].Meta)
	}
}

func TestImportTasksUsesParentOverrideForRootTasks(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	parent, err := CreateTask(root, CreateTaskInput{Title: "Existing parent", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}

	importPath := filepath.Join(root, "child-imports.json")
	importDoc := `{
  "tasks": [
    {
      "title": "Imported child",
      "type": "review",
      "body": "# Review\n"
    }
  ]
}`
	if err := os.WriteFile(importPath, []byte(importDoc), 0o644); err != nil {
		t.Fatalf("WriteFile import doc: %v", err)
	}

	result, err := ImportTasks(root, importPath, ImportTaskInput{ParentID: parent.ID})
	if err != nil {
		t.Fatalf("ImportTasks: %v", err)
	}

	task, err := GetTask(root, result.Primary.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	if task.Meta.ParentID != parent.ID {
		t.Fatalf("expected imported root to use override parent %q, got %q", parent.ID, task.Meta.ParentID)
	}
}

func TestCreateImportTemplateCopy(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	path, err := CreateImportTemplateCopy(root)
	if err != nil {
		t.Fatalf("CreateImportTemplateCopy: %v", err)
	}

	if filepath.Dir(path) != ImportsDir(root) {
		t.Fatalf("expected import copy in %s, got %s", ImportsDir(root), path)
	}

	templateData, err := os.ReadFile(ImportTemplatePath(root))
	if err != nil {
		t.Fatalf("ReadFile template: %v", err)
	}
	copyData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile copy: %v", err)
	}
	if string(copyData) != string(templateData) {
		t.Fatalf("expected import copy to match template")
	}
}

func TestLatestImportPath(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	first := filepath.Join(ImportsDir(root), "import-first.json")
	second := filepath.Join(ImportsDir(root), "import-second.json")
	if err := os.WriteFile(first, []byte(`{"tasks":[{"title":"First"}]}`), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(second, []byte(`{"tasks":[{"title":"Second"}]}`), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}

	oldTime := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Minute)
	if err := os.Chtimes(first, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes first: %v", err)
	}
	if err := os.Chtimes(second, newTime, newTime); err != nil {
		t.Fatalf("Chtimes second: %v", err)
	}

	path, err := LatestImportPath(root)
	if err != nil {
		t.Fatalf("LatestImportPath: %v", err)
	}
	if path != second {
		t.Fatalf("expected latest import path %s, got %s", second, path)
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

func TestDeleteCurrentTaskClearsCurrentWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	created, err := CreateTask(root, CreateTaskInput{Title: "Leaf", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task, err := GetTask(root, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if err := WriteCurrentWorkspace(root, task, CurrentWorkspaceInput{}); err != nil {
		t.Fatalf("WriteCurrentWorkspace: %v", err)
	}

	if err := DeleteTask(root, created.ID, false); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	context, err := CurrentContext(root)
	if err != nil {
		t.Fatalf("CurrentContext: %v", err)
	}
	if len(context) != 0 {
		t.Fatalf("expected current context to be cleared, got %#v", context)
	}

	workspacePath := filepath.Join(root, TaskerDirName, "current", "WORKSPACE.md")
	data, err := os.ReadFile(workspacePath)
	if err != nil {
		t.Fatalf("ReadFile workspace: %v", err)
	}
	if string(data) != workspaceTemplate() {
		t.Fatalf("expected workspace to reset, got:\n%s", string(data))
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
		!strings.Contains(rows[0], "[IN PROGRESS]") ||
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

	if err := writeJSON(filepath.Join(parent.Path, "status.json"), TaskStatus{
		Status:  "NEW",
		Agent:   "unknown",
		Started: "2026-06-29T18:55:00+03:00",
		Sessions: []TaskSession{
			{
				Agent:         "codex",
				ID:            "019f18b6-d117-7de2-ae23-4aa2ffaff8a1",
				Source:        "CODEX_THREAD_ID",
				RecordedAt:    "2026-06-29T18:55:00+03:00",
				ResumeCommand: "codex resume 019f18b6-d117-7de2-ae23-4aa2ffaff8a1",
				ForkCommand:   "codex fork 019f18b6-d117-7de2-ae23-4aa2ffaff8a1",
			},
		},
	}); err != nil {
		t.Fatalf("write parent status with session: %v", err)
	}

	rows, err := TaskStatusDetails(root, parent.ID)
	if err != nil {
		t.Fatalf("TaskStatusDetails: %v", err)
	}

	output := strings.Join(rows, "\n")
	for _, want := range []string{
		fmt.Sprintf("Task %s: Parent", parent.ID),
		"Status   [NEW]",
		"Sessions",
		"  codex 019f18b6-d117-7de2-ae23-4aa2ffaff8a1",
		"    resume  codex resume 019f18b6-d117-7de2-ae23-4aa2ffaff8a1",
		"    fork    codex fork 019f18b6-d117-7de2-ae23-4aa2ffaff8a1",
		"Subtasks",
		fmt.Sprintf("  %s  [IN PROGRESS]", child.ID),
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
	want := []string{"bug", "decision", "documentation", "feature", "research", "review", "test"}

	if len(got) != len(want) {
		t.Fatalf("expected %d task types, got %d", len(want), len(got))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected task type %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestValidTaskStatusesSorted(t *testing.T) {
	got := ValidTaskStatuses()
	want := []string{
		"AWAITING_ACTION",
		"BLOCKED",
		"DONE",
		"HANDOFF",
		"IN_PROGRESS",
		"NEW",
		"PLANNED",
		"REVIEW",
		"RUNNING",
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d task statuses, got %d", len(want), len(got))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected task status %d to be %q, got %q", i, want[i], got[i])
		}
	}
}

func TestTaskStatusesRejectInvalidStatus(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	task, err := CreateTask(root, CreateTaskInput{Title: "Broken", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := writeJSON(filepath.Join(task.Path, "status.json"), TaskStatus{
		Status:  "shipped",
		Agent:   "worker",
		Started: "2026-06-30T10:00:00+03:00",
	}); err != nil {
		t.Fatalf("write status: %v", err)
	}

	if _, err := TaskStatuses(root); err == nil || !strings.Contains(err.Error(), `invalid status "SHIPPED"`) {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func TestTaskStatusesRejectCompletedStatus(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	task, err := CreateTask(root, CreateTaskInput{Title: "Legacy", Type: "feature"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := writeJSON(filepath.Join(task.Path, "status.json"), TaskStatus{
		Status:  "completed",
		Agent:   "worker",
		Started: "2026-06-30T10:00:00+03:00",
	}); err != nil {
		t.Fatalf("write status: %v", err)
	}

	if _, err := TaskStatuses(root); err == nil || !strings.Contains(err.Error(), `invalid status "COMPLETED"`) {
		t.Fatalf("expected completed status error, got %v", err)
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

func TestDeleteParentRecursiveClearsCurrentChildWorkspace(t *testing.T) {
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

	childTask, err := GetTask(root, child.ID)
	if err != nil {
		t.Fatalf("GetTask child: %v", err)
	}
	if err := WriteCurrentWorkspace(root, childTask, CurrentWorkspaceInput{}); err != nil {
		t.Fatalf("WriteCurrentWorkspace: %v", err)
	}

	if err := DeleteTask(root, parent.ID, true); err != nil {
		t.Fatalf("DeleteTask recursive: %v", err)
	}

	context, err := CurrentContext(root)
	if err != nil {
		t.Fatalf("CurrentContext: %v", err)
	}
	if len(context) != 0 {
		t.Fatalf("expected current context to be cleared, got %#v", context)
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
		"Do not create task folders manually under .tasker/tasks; use `tasker new`, `tasker add`, or `tasker import`",
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

func TestCheckoutTaskForceBranchCreatesBranchWithoutCheckoutFlag(t *testing.T) {
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

	result, err := CheckoutTask(root, task.ID, CheckoutTaskInput{ForceBranch: true})
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
