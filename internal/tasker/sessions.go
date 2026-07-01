package tasker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TaskSession struct {
	Agent         string `json:"agent"`
	ID            string `json:"id"`
	Source        string `json:"source,omitempty"`
	RecordedAt    string `json:"recorded_at"`
	ResumeCommand string `json:"resume_command,omitempty"`
	ForkCommand   string `json:"fork_command,omitempty"`
}

type TaskSessionIndex struct {
	Sessions []TaskSession `json:"sessions"`
}

type persistedCodexExecSessionMatch struct {
	ID         string
	RecordedAt time.Time
}

type persistedCodexSessionMeta struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		SessionID  string `json:"session_id"`
		ID         string `json:"id"`
		Timestamp  string `json:"timestamp"`
		Cwd        string `json:"cwd"`
		Originator string `json:"originator"`
		Source     string `json:"source"`
	} `json:"payload"`
}

func DetectTaskSessions() []TaskSession {
	return detectTaskSessionsAt(time.Now())
}

func detectTaskSessionsAt(now time.Time) []TaskSession {
	sessions := make([]TaskSession, 0, 3)

	if session, ok := detectExplicitTaskerSession(now); ok {
		sessions = append(sessions, session)
	}

	if session, ok := detectCodexSession(now); ok {
		sessions = append(sessions, session)
	}

	if session, ok := detectClaudeSession(now); ok {
		sessions = append(sessions, session)
	}

	return uniqueTaskSessions(sessions)
}

func detectExplicitTaskerSession(now time.Time) (TaskSession, bool) {
	id, ok := lookupTrimmedEnv("TASKER_SESSION_ID")
	if !ok {
		return TaskSession{}, false
	}

	agent := "external"
	if value, ok := lookupTrimmedEnv("TASKER_SESSION_AGENT"); ok {
		agent = value
	}

	session := TaskSession{
		Agent:      agent,
		ID:         id,
		Source:     "TASKER_SESSION_ID",
		RecordedAt: now.Format(time.RFC3339),
	}
	if value, ok := lookupTrimmedEnv("TASKER_SESSION_RESUME_COMMAND"); ok {
		session.ResumeCommand = value
	}
	if value, ok := lookupTrimmedEnv("TASKER_SESSION_FORK_COMMAND"); ok {
		session.ForkCommand = value
	}

	return session, true
}

func detectCodexSession(now time.Time) (TaskSession, bool) {
	id, ok := lookupTrimmedEnv("CODEX_THREAD_ID")
	if !ok {
		return TaskSession{}, false
	}

	return newCodexTaskSession(id, "CODEX_THREAD_ID", now), true
}

func detectClaudeSession(now time.Time) (TaskSession, bool) {
	for _, envName := range []string{"CLAUDE_SESSION_ID", "CLAUDE_THREAD_ID", "ANTHROPIC_SESSION_ID"} {
		id, ok := lookupTrimmedEnv(envName)
		if !ok {
			continue
		}

		return TaskSession{
			Agent:      "claude",
			ID:         id,
			Source:     envName,
			RecordedAt: now.Format(time.RFC3339),
		}, true
	}

	return TaskSession{}, false
}

func lookupTrimmedEnv(name string) (string, bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	return value, true
}

func uniqueTaskSessions(sessions []TaskSession) []TaskSession {
	seen := make(map[string]struct{}, len(sessions))
	result := make([]TaskSession, 0, len(sessions))
	for _, session := range sessions {
		key := session.Agent + "\x00" + session.ID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, session)
	}
	return result
}

func newCodexTaskSession(id, source string, now time.Time) TaskSession {
	return TaskSession{
		Agent:         "codex",
		ID:            id,
		Source:        source,
		RecordedAt:    now.Format(time.RFC3339),
		ResumeCommand: "codex resume " + id,
		ForkCommand:   "codex fork " + id,
	}
}

func StoreTaskSession(task *Task, session TaskSession) error {
	task.Status.Sessions = uniqueTaskSessions(append(task.Status.Sessions, session))
	if err := writeJSON(filepath.Join(task.Path, "status.json"), task.Status); err != nil {
		return err
	}

	return writeTaskSessionIndex(task.Path, task.Status.Sessions)
}

func TaskLiveOutputPath(task *Task) string {
	return filepath.Join(task.Path, "sessions", "live-output.log")
}

func ResetTaskLiveOutput(task *Task) error {
	return os.WriteFile(TaskLiveOutputPath(task), nil, 0o644)
}

func ReadTaskLiveOutput(task *Task) (string, error) {
	data, err := os.ReadFile(TaskLiveOutputPath(task))
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", os.ErrNotExist
	}
	return content, nil
}

func writeTaskSessionIndex(taskPath string, sessions []TaskSession) error {
	return writeJSON(filepath.Join(taskPath, "sessions", "index.json"), TaskSessionIndex{
		Sessions: sessions,
	})
}

func findPersistedCodexExecSessionID(root string, notBefore time.Time) (string, bool, error) {
	match, ok, err := findPersistedCodexExecSession(root, notBefore)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, nil
	}
	return match.ID, true, nil
}

func findPersistedCodexExecSession(root string, notBefore time.Time) (persistedCodexExecSessionMatch, bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return persistedCodexExecSessionMatch{}, false, err
	}

	sessionsRoot := filepath.Join(homeDir, ".codex", "sessions")
	if _, err := os.Stat(sessionsRoot); err != nil {
		if os.IsNotExist(err) {
			return persistedCodexExecSessionMatch{}, false, nil
		}
		return persistedCodexExecSessionMatch{}, false, err
	}

	rootKey := normalizeSessionCWD(root)
	best := persistedCodexExecSessionMatch{}

	err = filepath.WalkDir(sessionsRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}

		sessionID, recordedAt, cwd, isExec, err := readPersistedCodexSessionMeta(path)
		if err != nil || !isExec || sessionID == "" {
			return nil
		}
		if recordedAt.Before(notBefore) {
			return nil
		}
		if normalizeSessionCWD(cwd) != rootKey {
			return nil
		}
		if best.ID == "" || recordedAt.After(best.RecordedAt) {
			best = persistedCodexExecSessionMatch{
				ID:         sessionID,
				RecordedAt: recordedAt,
			}
		}
		return nil
	})
	if err != nil {
		return persistedCodexExecSessionMatch{}, false, err
	}
	if best.ID == "" {
		return persistedCodexExecSessionMatch{}, false, nil
	}
	return best, true, nil
}

func readPersistedCodexSessionMeta(path string) (string, time.Time, string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", time.Time{}, "", false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", time.Time{}, "", false, err
		}
		return "", time.Time{}, "", false, nil
	}

	var meta persistedCodexSessionMeta
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		return "", time.Time{}, "", false, err
	}
	if meta.Type != "session_meta" {
		return "", time.Time{}, "", false, nil
	}

	sessionID := strings.TrimSpace(meta.Payload.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(meta.Payload.ID)
	}
	recordedAt, _ := time.Parse(time.RFC3339, strings.TrimSpace(meta.Payload.Timestamp))
	if recordedAt.IsZero() {
		recordedAt, _ = time.Parse(time.RFC3339, strings.TrimSpace(meta.Timestamp))
	}

	isExec := strings.EqualFold(strings.TrimSpace(meta.Payload.Originator), "codex_exec") ||
		strings.EqualFold(strings.TrimSpace(meta.Payload.Source), "exec")

	return sessionID, recordedAt, meta.Payload.Cwd, isExec, nil
}

func normalizeSessionCWD(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

type persistedCodexEventMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type persistedCodexResponseItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type persistedCodexSessionEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func ReadTaskAgentOutput(task *Task) (TaskSession, string, error) {
	for i := len(task.Status.Sessions) - 1; i >= 0; i-- {
		session := task.Status.Sessions[i]
		if strings.TrimSpace(strings.ToLower(session.Agent)) != "codex" {
			continue
		}

		output, err := ReadCodexSessionOutput(session.ID)
		if err == nil {
			return session, output, nil
		}
		if !os.IsNotExist(err) {
			return session, "", err
		}
	}

	session, output, err := ReadTaskExecutionOutput(task)
	if err == nil {
		return session, output, nil
	}
	if !os.IsNotExist(err) {
		return session, "", err
	}

	output, err = ReadTaskLiveOutput(task)
	if err == nil {
		return TaskSession{Agent: "codex", Source: "task live output"}, output, nil
	}
	if !os.IsNotExist(err) {
		return TaskSession{Agent: "codex", Source: "task live output"}, "", err
	}

	return TaskSession{}, "", os.ErrNotExist
}

func ReadTaskExecutionOutput(task *Task) (TaskSession, string, error) {
	state, err := ReadTaskExecutionState(task)
	if err != nil {
		return TaskSession{}, "", err
	}
	startedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(state.StartedAt))
	if err != nil {
		return TaskSession{}, "", err
	}

	root, err := FindWorkspaceRoot(task.Path)
	if err != nil {
		return TaskSession{}, "", err
	}

	match, ok, err := findPersistedCodexExecSession(root, startedAt)
	if err != nil {
		return TaskSession{}, "", err
	}
	if !ok {
		return TaskSession{}, "", os.ErrNotExist
	}

	output, err := ReadCodexSessionOutput(match.ID)
	if err != nil {
		return newCodexTaskSession(match.ID, "codex exec session file", match.RecordedAt), "", err
	}
	return newCodexTaskSession(match.ID, "codex exec session file", match.RecordedAt), output, nil
}

func ReadCodexSessionOutput(sessionID string) (string, error) {
	path, err := findPersistedCodexSessionPath(sessionID)
	if err != nil {
		return "", err
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	lines := make([]string, 0, 32)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event persistedCodexSessionEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		switch event.Type {
		case "event_msg":
			var payload persistedCodexEventMessage
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			if payload.Type == "agent_message" {
				appendTranscriptLine(&lines, payload.Message)
			}
		case "response_item":
			var payload persistedCodexResponseItem
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			if payload.Type != "message" || payload.Role != "assistant" {
				continue
			}
			for _, content := range payload.Content {
				if content.Type == "output_text" {
					appendTranscriptLine(&lines, content.Text)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", os.ErrNotExist
	}
	return strings.Join(lines, "\n\n"), nil
}

func appendTranscriptLine(lines *[]string, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	if len(*lines) > 0 && (*lines)[len(*lines)-1] == trimmed {
		return
	}
	*lines = append(*lines, trimmed)
}

func findPersistedCodexSessionPath(sessionID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	sessionsRoot := filepath.Join(homeDir, ".codex", "sessions")
	if _, err := os.Stat(sessionsRoot); err != nil {
		return "", err
	}

	suffix := "-" + strings.TrimSpace(sessionID) + ".jsonl"
	found := ""
	err = filepath.WalkDir(sessionsRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		if strings.HasSuffix(filepath.Base(path), suffix) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("%w: codex session %s", os.ErrNotExist, sessionID)
	}
	return found, nil
}
