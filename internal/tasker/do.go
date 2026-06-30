package tasker

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var execCommand = exec.Command
var taskerDoProgressTicker = time.NewTicker

type codexExecEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type codexSessionMetaPayload struct {
	SessionID string `json:"session_id"`
	ID        string `json:"id"`
}

type codexEventMessagePayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type codexResponseItemPayload struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func DoTask(root, id string, out, errOut io.Writer) error {
	task, err := GetTask(root, id)
	if err != nil {
		return err
	}

	startedAt := time.Now()
	if err := UpdateTaskStatus(task, "RUNNING", "codex", startedAt); err != nil {
		return err
	}
	if err := WriteCurrentWorkspace(root, task, CurrentWorkspaceInput{}); err != nil {
		return err
	}
	cmd := execCommand("codex",
		"exec",
		"--json",
		"--cd", root,
		buildTaskerDoPrompt(task),
	)
	cmd.Dir = root

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer devNull.Close()
	cmd.Stdin = devNull

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	progress := newTaskerDoProgress(out)
	progress.start()
	defer progress.stop()

	stderrDone := make(chan error, 1)
	go func() {
		stderrDone <- relayCodexExecStderr(stderr, progress, errOut)
	}()

	decoder := json.NewDecoder(bufio.NewReader(stdout))
	sessionStored := false
	storeSession := func(sessionID, source string) error {
		progress.stop()
		if err := StoreTaskSession(task, newCodexTaskSession(sessionID, source, time.Now())); err != nil {
			return err
		}
		sessionStored = true
		fmt.Fprintf(out, "Started Codex session: %s\n", sessionID)
		return nil
	}

	for {
		var event codexExecEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			progress.stop()
			_ = cmd.Wait()
			<-stderrDone
			return err
		}

		if !sessionStored {
			if sessionID, ok := sessionIDFromEvent(event); ok {
				if err := storeSession(sessionID, "codex exec stdout"); err != nil {
					_ = cmd.Wait()
					return err
				}
			}
		}

		renderCodexExecEvent(out, progress, event)
	}

	if err := cmd.Wait(); err != nil {
		progress.stop()
		<-stderrDone
		return err
	}
	progress.stop()
	if err := <-stderrDone; err != nil {
		return err
	}
	if !sessionStored {
		sessionID, ok, err := findPersistedCodexExecSessionID(root, startedAt)
		if err != nil {
			return err
		}
		if ok {
			if err := storeSession(sessionID, "codex exec session file"); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("codex exec completed without exposing a session id on stdout or in ~/.codex/sessions")
		}
	}

	return finalizeDoTaskStatus(root, task.Meta.ID, startedAt)
}

func buildTaskerDoPrompt(task *Task) string {
	return strings.TrimSpace(fmt.Sprintf(`
Continue Tasker task %s: %s.

Work from the repository root and follow the repository instructions in AGENTS.md, .tasker/START.md, .tasker/instructions.md, and .tasker/current/WORKSPACE.md before changing code.

Read the current task folder at %s and complete the task end-to-end.

Update declaration.md for in-progress findings, update result.md with the final outcome, and update status.json to the final task status before you finish.
`, task.Meta.ID, task.Meta.Title, filepath.ToSlash(task.Path)))
}

func finalizeDoTaskStatus(root, id string, startedAt time.Time) error {
	task, err := GetTask(root, id)
	if err != nil {
		return err
	}

	switch task.Status.Status {
	case "BLOCKED", "AWAITING_ACTION", "HANDOFF", "REVIEW", "DONE":
		return nil
	case "NEW", "RUNNING":
		return UpdateTaskStatus(task, "DONE", "codex", startedAt)
	default:
		return nil
	}
}

func sessionIDFromEvent(event codexExecEvent) (string, bool) {
	if event.Type != "session_meta" {
		return "", false
	}

	var payload codexSessionMetaPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "", false
	}
	if value := strings.TrimSpace(payload.SessionID); value != "" {
		return value, true
	}
	if value := strings.TrimSpace(payload.ID); value != "" {
		return value, true
	}
	return "", false
}

func renderCodexExecEvent(out io.Writer, progress *taskerDoProgress, event codexExecEvent) {
	switch event.Type {
	case "event_msg":
		var payload codexEventMessagePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		if payload.Type == "agent_message" && strings.TrimSpace(payload.Message) != "" {
			progress.stop()
			fmt.Fprintln(out, payload.Message)
		}
	case "response_item":
		var payload codexResponseItemPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		if payload.Type != "message" || payload.Role != "assistant" {
			return
		}
		for _, content := range payload.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				progress.stop()
				fmt.Fprintln(out, content.Text)
			}
		}
	}
}

func relayCodexExecStderr(stderr io.Reader, progress *taskerDoProgress, errOut io.Writer) error {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		if shouldSuppressCodexExecStderr(line) {
			continue
		}
		progress.stop()
		fmt.Fprintln(errOut, line)
	}
	return scanner.Err()
}

func shouldSuppressCodexExecStderr(line string) bool {
	normalized := strings.TrimSpace(strings.ToLower(line))
	return strings.Contains(normalized, "reading additional input from stdin")
}

type taskerDoProgress struct {
	out      io.Writer
	done     chan struct{}
	finished chan struct{}
	stopOnce sync.Once
}

func newTaskerDoProgress(out io.Writer) *taskerDoProgress {
	return &taskerDoProgress{
		out:      out,
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (p *taskerDoProgress) start() {
	go func() {
		defer close(p.finished)

		frames := []string{
			"Tasker do is working.",
			"Tasker do is working..",
			"Tasker do is working...",
			"Tasker do is working....",
		}
		ticker := taskerDoProgressTicker(250 * time.Millisecond)
		defer ticker.Stop()

		i := 0
		fmt.Fprintf(p.out, "\r%s", frames[i])
		i++

		for {
			select {
			case <-p.done:
				fmt.Fprint(p.out, "\r\033[K")
				return
			case <-ticker.C:
				fmt.Fprintf(p.out, "\r%s", frames[i%len(frames)])
				i++
			}
		}
	}()
}

func (p *taskerDoProgress) stop() {
	p.stopOnce.Do(func() {
		close(p.done)
		<-p.finished
	})
}
