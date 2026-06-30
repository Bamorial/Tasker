package tasker

import (
	"bufio"
	"encoding/json"
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

func writeTaskSessionIndex(taskPath string, sessions []TaskSession) error {
	return writeJSON(filepath.Join(taskPath, "sessions", "index.json"), TaskSessionIndex{
		Sessions: sessions,
	})
}

func findPersistedCodexExecSessionID(root string, notBefore time.Time) (string, bool, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}

	sessionsRoot := filepath.Join(homeDir, ".codex", "sessions")
	if _, err := os.Stat(sessionsRoot); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}

	rootKey := normalizeSessionCWD(root)
	bestID := ""
	var bestTime time.Time

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
		if bestID == "" || recordedAt.After(bestTime) {
			bestID = sessionID
			bestTime = recordedAt
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if bestID == "" {
		return "", false, nil
	}
	return bestID, true, nil
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
