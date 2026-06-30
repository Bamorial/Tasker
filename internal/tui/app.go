package tui

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bamorial/tasker/internal/buildinfo"
	"github.com/bamorial/tasker/internal/tasker"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tab int

const (
	tabDashboard tab = iota
	tabTasks
	tabCurrent
	tabJobs
)

type modalKind string

const (
	modalNone                 modalKind = ""
	modalNewTask              modalKind = "new-task"
	modalAddTask              modalKind = "add-task"
	modalMeta                 modalKind = "meta"
	modalCheckout             modalKind = "checkout"
	modalImport               modalKind = "import"
	modalOpenDoc              modalKind = "open-doc"
	modalDelete               modalKind = "delete"
	modalSession              modalKind = "session"
	filterAll                           = "all"
	projectInstructionsTarget           = "project-instructions"
)

type sessionModal struct {
	Action   tasker.AgentSessionAction
	Sessions []tasker.TaskSession
	Index    int
}

type confirmModal struct {
	Title       string
	Body        string
	Recursive   bool
	SelectedYes bool
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type jobState struct {
	Label   string
	TaskID  string
	Started time.Time
	Buffer  *safeBuffer
	Output  string
	Running bool
}

type mutationResultMsg struct {
	Status     string
	SelectedID string
	Err        error
	Exec       *exec.Cmd
	Refresh    bool
}

type snapshotMsg struct {
	Snapshot *tasker.WorkspaceSnapshot
	Err      error
}

type jobFinishedMsg struct {
	Status string
	TaskID string
	Err    error
}

type jobTickMsg struct{}

type externalDoneMsg struct {
	Status     string
	SelectedID string
	Err        error
	Refresh    bool
}

type model struct {
	root string

	width  int
	height int

	tab  tab
	help bool

	snapshot *tasker.WorkspaceSnapshot

	filterInput    textinput.Model
	filtering      bool
	statusFilter   string
	typeFilter     string
	filtered       []tasker.TaskTreeItem
	selectedTaskID string

	detailViewport viewport.Model
	mainViewport   viewport.Model
	jobViewport    viewport.Model

	form    *formModal
	session *sessionModal
	confirm *confirmModal

	activeJob        *jobState
	lastStatus       string
	lastErr          string
	pendingSelection string
}

func Run(root string) error {
	m := newModel(root)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newModel(root string) model {
	filter := textinput.New()
	filter.Prompt = "Filter: "
	filter.Placeholder = "title, id, slug"
	filter.Width = 24

	detail := viewport.New(0, 0)
	main := viewport.New(0, 0)
	job := viewport.New(0, 0)

	return model{
		root:           root,
		tab:            tabTasks,
		filterInput:    filter,
		statusFilter:   filterAll,
		typeFilter:     filterAll,
		detailViewport: detail,
		mainViewport:   main,
		jobViewport:    job,
	}
}

func (m model) Init() tea.Cmd {
	return refreshCmd(m.root)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		m.syncDerivedState()
		return m, nil
	case snapshotMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
			return m, nil
		}
		m.snapshot = msg.Snapshot
		m.lastErr = ""
		m.syncDerivedState()
		if m.pendingSelection != "" {
			m.selectedTaskID = m.pendingSelection
			m.pendingSelection = ""
			m.ensureSelection()
			m.syncDerivedState()
		}
		return m, nil
	case mutationResultMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
			return m, nil
		}
		m.lastStatus = msg.Status
		m.lastErr = ""
		m.pendingSelection = msg.SelectedID
		if msg.Exec != nil {
			return m, tea.ExecProcess(msg.Exec, func(err error) tea.Msg {
				return externalDoneMsg{
					Status:     msg.Status,
					SelectedID: msg.SelectedID,
					Err:        err,
					Refresh:    msg.Refresh,
				}
			})
		}
		if msg.Refresh {
			return m, refreshCmd(m.root)
		}
		return m, nil
	case externalDoneMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
			return m, nil
		}
		m.lastStatus = msg.Status
		m.lastErr = ""
		m.pendingSelection = msg.SelectedID
		if msg.Refresh {
			return m, refreshCmd(m.root)
		}
		return m, nil
	case jobFinishedMsg:
		if m.activeJob != nil && m.activeJob.TaskID == msg.TaskID {
			m.activeJob.Running = false
			m.activeJob.Output = m.activeJob.Buffer.String()
			m.jobViewport.SetContent(m.activeJob.Output)
		}
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
			return m, refreshCmd(m.root)
		}
		m.lastStatus = msg.Status
		m.lastErr = ""
		return m, refreshCmd(m.root)
	case jobTickMsg:
		if m.activeJob != nil && m.activeJob.Running {
			m.activeJob.Output = m.activeJob.Buffer.String()
			m.jobViewport.SetContent(m.activeJob.Output)
			return m, pollJobCmd()
		}
		return m, nil
	case tea.KeyMsg:
		if m.form != nil {
			return m.updateForm(msg)
		}
		if m.session != nil {
			return m.updateSessionPicker(msg)
		}
		if m.confirm != nil {
			return m.updateConfirm(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.updateKeys(msg)
	}

	var cmd tea.Cmd
	switch m.tab {
	case tabTasks:
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	case tabCurrent, tabDashboard:
		m.mainViewport, cmd = m.mainViewport.Update(msg)
		return m, cmd
	case tabJobs:
		m.jobViewport, cmd = m.jobViewport.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading Tasker TUI..."
	}

	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()

	view := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	if m.help {
		return overlay(view, m.renderHelp())
	}
	if m.form != nil {
		return overlay(view, m.renderForm())
	}
	if m.session != nil {
		return overlay(view, m.renderSessionPicker())
	}
	if m.confirm != nil {
		return overlay(view, m.renderConfirm())
	}
	return view
}

func (m *model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % 4
		m.syncDerivedState()
		return m, nil
	case "shift+tab":
		m.tab = (m.tab - 1 + 4) % 4
		m.syncDerivedState()
		return m, nil
	case "?":
		m.help = !m.help
		return m, nil
	case "r":
		return m, refreshCmd(m.root)
	case "/":
		m.filtering = true
		m.filterInput.Focus()
		return m, nil
	case "S":
		m.statusFilter = cycleOption(m.statusFilter, append([]string{filterAll}, tasker.ValidTaskStatuses()...))
		m.applyFilters()
		return m, nil
	case "T":
		m.typeFilter = cycleOption(m.typeFilter, append([]string{filterAll}, tasker.ValidTaskTypes()...))
		m.applyFilters()
		return m, nil
	}

	switch m.tab {
	case tabTasks:
		return m.updateTasksViewKeys(msg)
	case tabDashboard, tabCurrent:
		return m.updateViewportKeys(msg, &m.mainViewport)
	case tabJobs:
		return m.updateViewportKeys(msg, &m.jobViewport)
	default:
		return m, nil
	}
}

func (m *model) updateTasksViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.moveSelection(-1)
		return m, nil
	case "down", "j":
		m.moveSelection(1)
		return m, nil
	case "pgup":
		m.moveSelection(-5)
		return m, nil
	case "pgdown":
		m.moveSelection(5)
		return m, nil
	case "n":
		m.form = m.newTaskForm()
		return m, nil
	case "a":
		m.form = m.newChildTaskForm()
		return m, nil
	case "m":
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.form = m.newMetaForm(*task)
		return m, nil
	case "c":
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.form = m.newCheckoutForm(*task)
		return m, nil
	case "u":
		m.form = m.newImportForm()
		return m, nil
	case "I":
		return m, m.createImportTemplateCmd()
	case "d":
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.confirm = &confirmModal{
			Title:     "Delete Task",
			Body:      fmt.Sprintf("Delete task %s %s?", task.Meta.ID, task.Meta.Title),
			Recursive: false,
		}
		return m, nil
	case "e":
		m.form = m.newOpenDocForm()
		return m, nil
	case "x":
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		return m.startDoJob(*task)
	case "s":
		return m.openSessionPicker(tasker.AgentSessionResume)
	case "f":
		return m.openSessionPicker(tasker.AgentSessionFork)
	case "enter":
		m.tab = tabCurrent
		m.syncDerivedState()
		return m, nil
	default:
		return m.updateViewportKeys(msg, &m.detailViewport)
	}
}

func (m *model) updateViewportKeys(msg tea.KeyMsg, vp *viewport.Model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "up", "k", "ctrl+p":
		vp.LineUp(1)
	case "down", "j", "ctrl+n":
		vp.LineDown(1)
	case "pgup", "b":
		vp.HalfViewUp()
	case "pgdown", "space":
		vp.HalfViewDown()
	case "g", "home":
		vp.GotoTop()
	case "G", "end":
		vp.GotoBottom()
	default:
		*vp, cmd = vp.Update(msg)
	}
	return m, cmd
}

func (m *model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	case "enter":
		m.filtering = false
		m.filterInput.Blur()
		m.applyFilters()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.applyFilters()
	return m, cmd
}

func (m *model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.form == nil {
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.form = nil
		return m, nil
	case "tab", "down":
		m.form.Next()
		return m, nil
	case "shift+tab", "up":
		m.form.Prev()
		return m, nil
	case "left":
		if len(m.form.Fields) > 0 {
			m.form.Fields[m.form.Focus].Cycle(-1)
		}
		return m, nil
	case "right":
		if len(m.form.Fields) > 0 {
			m.form.Fields[m.form.Focus].Cycle(1)
		}
		return m, nil
	case "ctrl+s":
		return m.submitForm()
	case "enter":
		if len(m.form.Fields) > 0 && m.form.Fields[m.form.Focus].Kind == fieldText {
			m.form.Next()
			return m, nil
		}
		return m.submitForm()
	}

	if len(m.form.Fields) == 0 {
		return m, nil
	}

	field := &m.form.Fields[m.form.Focus]
	if field.Kind != fieldText {
		return m, nil
	}

	var cmd tea.Cmd
	field.Input, cmd = field.Input.Update(msg)
	return m, cmd
}

func (m *model) updateSessionPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.session = nil
		return m, nil
	case "up", "k":
		if m.session.Index > 0 {
			m.session.Index--
		}
		return m, nil
	case "down", "j":
		if m.session.Index < len(m.session.Sessions)-1 {
			m.session.Index++
		}
		return m, nil
	case "enter":
		task := m.selectedTask()
		if task == nil {
			m.session = nil
			return m.withError("select a task first"), nil
		}
		selected := m.session.Sessions[m.session.Index]
		cmd, err := tasker.TaskSessionCommand(task, selected, m.session.Action)
		if err != nil {
			m.session = nil
			return m.withError(err.Error()), nil
		}
		status := fmt.Sprintf("%s session %s", strings.Title(string(m.session.Action)), selected.ID)
		m.session = nil
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return externalDoneMsg{
				Status:     status,
				SelectedID: task.Meta.ID,
				Err:        err,
				Refresh:    true,
			}
		})
	}
	return m, nil
}

func (m *model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.confirm = nil
		return m, nil
	case "left", "h", "right", "l":
		m.confirm.SelectedYes = !m.confirm.SelectedYes
		return m, nil
	case "r":
		m.confirm.Recursive = !m.confirm.Recursive
		return m, nil
	case "enter":
		if !m.confirm.SelectedYes {
			m.confirm = nil
			return m, nil
		}
		task := m.selectedTask()
		if task == nil {
			m.confirm = nil
			return m.withError("select a task first"), nil
		}
		recursive := m.confirm.Recursive
		selectedID := task.Meta.ID
		m.confirm = nil
		return m, mutationCmd("Deleted task "+selectedID, selectedID, true, func() (*exec.Cmd, error) {
			return nil, tasker.DeleteTask(m.root, selectedID, recursive)
		})
	}
	return m, nil
}

func (m *model) submitForm() (tea.Model, tea.Cmd) {
	if m.form == nil {
		return m, nil
	}
	values := m.form.Values()
	kind := m.form.Kind
	m.form = nil

	switch kind {
	case modalNewTask:
		return m, m.submitNewTask(values)
	case modalAddTask:
		return m, m.submitAddTask(values)
	case modalMeta:
		return m, m.submitMeta(values)
	case modalCheckout:
		return m, m.submitCheckout(values)
	case modalImport:
		return m, m.submitImport(values)
	case modalOpenDoc:
		return m, m.submitOpenDoc(values)
	default:
		return m, nil
	}
}

func (m *model) submitNewTask(values map[string]string) tea.Cmd {
	openRequested := values["open_behavior"] == "open"
	return func() tea.Msg {
		created, err := tasker.CreateTask(m.root, tasker.CreateTaskInput{
			Title: values["title"],
			Type:  values["type"],
		})
		if err != nil {
			return mutationResultMsg{Err: err}
		}

		status := "Created task " + created.ID
		if mode := values["checkout_mode"]; mode == "checkout" || mode == "branch-checkout" {
			_, err = tasker.CheckoutTask(m.root, created.ID, tasker.CheckoutTaskInput{
				NoBranch:    mode != "branch-checkout",
				ForceBranch: mode == "branch-checkout",
			})
			if err != nil {
				return mutationResultMsg{Err: err}
			}
			status = "Created and checked out task " + created.ID
		}

		var cmd *exec.Cmd
		if openRequested {
			path, err := tasker.TaskDocumentPath(created.Path, values["open_target"])
			if err != nil {
				return mutationResultMsg{Err: err}
			}
			cmd, err = tasker.EditorCommand(m.root, path)
			if err != nil {
				return mutationResultMsg{
					Status:     status + " (editor unavailable, see task files on disk)",
					SelectedID: created.ID,
					Refresh:    true,
				}
			}
		}

		return mutationResultMsg{
			Status:     status,
			SelectedID: created.ID,
			Exec:       cmd,
			Refresh:    true,
		}
	}
}

func (m *model) submitAddTask(values map[string]string) tea.Cmd {
	openRequested := values["open_behavior"] == "open"
	return func() tea.Msg {
		created, err := tasker.CreateTask(m.root, tasker.CreateTaskInput{
			Title:    values["title"],
			Type:     values["type"],
			ParentID: values["parent_id"],
		})
		if err != nil {
			return mutationResultMsg{Err: err}
		}

		var cmd *exec.Cmd
		if openRequested {
			path, err := tasker.TaskDocumentPath(created.Path, values["open_target"])
			if err != nil {
				return mutationResultMsg{Err: err}
			}
			cmd, err = tasker.EditorCommand(m.root, path)
			if err != nil {
				return mutationResultMsg{
					Status:     "Created child task " + created.ID + " (editor unavailable)",
					SelectedID: created.ID,
					Refresh:    true,
				}
			}
		}
		return mutationResultMsg{
			Status:     "Created child task " + created.ID,
			SelectedID: created.ID,
			Exec:       cmd,
			Refresh:    true,
		}
	}
}

func (m *model) submitMeta(values map[string]string) tea.Cmd {
	task := m.selectedTask()
	if task == nil {
		return errorCmd(fmt.Errorf("select a task first"))
	}

	openRequested := values["open_behavior"] == "open"
	selectedID := task.Meta.ID
	return func() tea.Msg {
		updated, err := tasker.UpdateTaskMeta(m.root, selectedID, tasker.UpdateTaskMetaInput{
			Title: values["title"],
			Type:  values["type"],
		})
		if err != nil {
			return mutationResultMsg{Err: err}
		}

		var cmd *exec.Cmd
		if openRequested {
			cmd, err = tasker.EditorCommand(m.root, updated.MetaFile)
			if err != nil {
				return mutationResultMsg{
					Status:     "Updated metadata for " + selectedID + " (editor unavailable)",
					SelectedID: selectedID,
					Refresh:    true,
				}
			}
		}

		return mutationResultMsg{
			Status:     "Updated metadata for " + selectedID,
			SelectedID: selectedID,
			Exec:       cmd,
			Refresh:    true,
		}
	}
}

func (m *model) submitCheckout(values map[string]string) tea.Cmd {
	task := m.selectedTask()
	if task == nil {
		return errorCmd(fmt.Errorf("select a task first"))
	}
	selectedID := task.Meta.ID
	mode := values["mode"]
	branch := values["branch"]

	return func() tea.Msg {
		input := tasker.CheckoutTaskInput{}
		switch mode {
		case "no-branch":
			input.NoBranch = true
		case "custom-branch":
			input.Branch = branch
		case "existing-branch":
			input.ExistingBranch = branch
		}

		if _, err := tasker.CheckoutTask(m.root, selectedID, input); err != nil {
			return mutationResultMsg{Err: err}
		}
		return mutationResultMsg{
			Status:     "Checked out task " + selectedID,
			SelectedID: selectedID,
			Refresh:    true,
		}
	}
}

func (m *model) submitImport(values map[string]string) tea.Cmd {
	openRequested := values["open_behavior"] == "open"
	return func() tea.Msg {
		importPath := values["path"]
		if strings.TrimSpace(importPath) == "" {
			latest, err := tasker.LatestImportPath(m.root)
			if err != nil {
				return mutationResultMsg{Err: err}
			}
			importPath = latest
		}

		result, err := tasker.ImportTasks(m.root, importPath, tasker.ImportTaskInput{
			ParentID: values["parent_id"],
		})
		if err != nil {
			return mutationResultMsg{Err: err}
		}

		selectedID := ""
		status := fmt.Sprintf("Imported %d tasks", len(result.Created))
		if result.Primary != nil {
			selectedID = result.Primary.ID
			if mode := values["checkout_mode"]; mode == "checkout" || mode == "branch-checkout" {
				_, err = tasker.CheckoutTask(m.root, result.Primary.ID, tasker.CheckoutTaskInput{
					NoBranch:    mode != "branch-checkout",
					ForceBranch: mode == "branch-checkout",
				})
				if err != nil {
					return mutationResultMsg{Err: err}
				}
			}
		}

		var cmd *exec.Cmd
		if openRequested && result.Primary != nil {
			path, err := tasker.TaskDocumentPath(result.Primary.Path, values["open_target"])
			if err != nil {
				return mutationResultMsg{Err: err}
			}
			cmd, err = tasker.EditorCommand(m.root, path)
			if err != nil {
				return mutationResultMsg{
					Status:     status + " (editor unavailable)",
					SelectedID: selectedID,
					Refresh:    true,
				}
			}
		}

		return mutationResultMsg{
			Status:     status,
			SelectedID: selectedID,
			Exec:       cmd,
			Refresh:    true,
		}
	}
}

func (m *model) submitOpenDoc(values map[string]string) tea.Cmd {
	target := values["target"]
	var selectedID string

	return func() tea.Msg {
		var path string
		switch target {
		case projectInstructionsTarget:
			path = filepath.Join(m.root, tasker.TaskerDirName, "instructions.md")
		default:
			task := m.selectedTask()
			if task == nil {
				return mutationResultMsg{Err: fmt.Errorf("select a task first")}
			}
			selectedID = task.Meta.ID
			var err error
			path, err = tasker.TaskDocumentPath(task.Path, target)
			if err != nil {
				return mutationResultMsg{Err: err}
			}
		}

		cmd, err := tasker.EditorCommand(m.root, path)
		if err != nil {
			return mutationResultMsg{
				Status:     "Editor unavailable, file path: " + path,
				SelectedID: selectedID,
			}
		}

		return mutationResultMsg{
			Status:     "Opened " + path,
			SelectedID: selectedID,
			Exec:       cmd,
		}
	}
}

func (m *model) createImportTemplateCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := tasker.CreateImportTemplateCopy(m.root)
		if err != nil {
			return mutationResultMsg{Err: err}
		}
		cmd, err := tasker.EditorCommand(m.root, path)
		if err != nil {
			return mutationResultMsg{Status: "Created import template at " + path}
		}
		return mutationResultMsg{
			Status: "Created import template " + path,
			Exec:   cmd,
		}
	}
}

func (m *model) openSessionPicker(action tasker.AgentSessionAction) (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m.withError("select a task first"), nil
	}
	sessions := tasker.SessionsForAction(task, action)
	if len(sessions) == 0 {
		return m.withError(fmt.Sprintf("task %s has no %s-capable stored sessions", task.Meta.ID, action)), nil
	}
	if len(sessions) == 1 {
		cmd, err := tasker.TaskSessionCommand(task, sessions[0], action)
		if err != nil {
			return m.withError(err.Error()), nil
		}
		status := fmt.Sprintf("%s session %s", strings.Title(string(action)), sessions[0].ID)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return externalDoneMsg{
				Status:     status,
				SelectedID: task.Meta.ID,
				Err:        err,
				Refresh:    true,
			}
		})
	}

	m.session = &sessionModal{
		Action:   action,
		Sessions: sessions,
	}
	return m, nil
}

func (m *model) startDoJob(task tasker.Task) (tea.Model, tea.Cmd) {
	if m.activeJob != nil && m.activeJob.Running {
		return m.withError("wait for the current `do` job to finish"), nil
	}

	buffer := &safeBuffer{}
	m.activeJob = &jobState{
		Label:   "tasker do " + task.Meta.ID,
		TaskID:  task.Meta.ID,
		Started: time.Now(),
		Buffer:  buffer,
		Running: true,
	}
	m.tab = tabJobs
	m.jobViewport.SetContent("")

	cmd := func() tea.Msg {
		err := tasker.DoTask(m.root, task.Meta.ID, io.MultiWriter(buffer), io.MultiWriter(buffer))
		return jobFinishedMsg{
			Status: "Finished tasker do for " + task.Meta.ID,
			TaskID: task.Meta.ID,
			Err:    err,
		}
	}
	return m, tea.Batch(cmd, pollJobCmd())
}

func (m *model) selectedTask() *tasker.Task {
	if m.snapshot == nil || m.selectedTaskID == "" {
		return nil
	}
	for i := range m.snapshot.Tasks {
		if m.snapshot.Tasks[i].Meta.ID == m.selectedTaskID {
			return &m.snapshot.Tasks[i]
		}
	}
	return nil
}

func (m *model) moveSelection(delta int) {
	if len(m.filtered) == 0 {
		m.selectedTaskID = ""
		return
	}

	index := 0
	for i, item := range m.filtered {
		if item.Task.Meta.ID == m.selectedTaskID {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(m.filtered) {
		index = len(m.filtered) - 1
	}
	m.selectedTaskID = m.filtered[index].Task.Meta.ID
	m.syncDerivedState()
}

func (m *model) applyFilters() {
	if m.snapshot == nil {
		m.filtered = nil
		return
	}

	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	filtered := make([]tasker.TaskTreeItem, 0, len(m.snapshot.Tree))
	for _, item := range m.snapshot.Tree {
		if m.statusFilter != filterAll && item.Task.Status.Status != m.statusFilter {
			continue
		}
		if m.typeFilter != filterAll && item.Task.Meta.Type != m.typeFilter {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{
				item.Task.Meta.ID,
				item.Task.Meta.Title,
				item.Task.Meta.Slug,
				item.Task.Status.Status,
				item.Task.Meta.Type,
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	m.filtered = filtered
	m.ensureSelection()
}

func (m *model) ensureSelection() {
	if len(m.filtered) == 0 {
		m.selectedTaskID = ""
		return
	}
	for _, item := range m.filtered {
		if item.Task.Meta.ID == m.selectedTaskID {
			return
		}
	}

	if m.snapshot != nil && m.snapshot.Current.Task != nil {
		for _, item := range m.filtered {
			if item.Task.Meta.ID == m.snapshot.Current.Task.Meta.ID {
				m.selectedTaskID = item.Task.Meta.ID
				return
			}
		}
	}

	m.selectedTaskID = m.filtered[0].Task.Meta.ID
}

func (m *model) syncDerivedState() {
	m.applyFilters()
	m.resizeViewports()
	m.syncDetailViewport()
	m.syncMainViewport()
	if m.activeJob != nil {
		m.activeJob.Output = m.activeJob.Buffer.String()
		m.jobViewport.SetContent(m.activeJob.Output)
	}
}

func (m *model) resizeViewports() {
	bodyHeight := maxInt(8, m.height-6)
	leftWidth := maxInt(24, m.width/2-2)
	rightWidth := maxInt(30, m.width-leftWidth-5)

	m.detailViewport.Width = rightWidth
	m.detailViewport.Height = bodyHeight - 5
	m.mainViewport.Width = maxInt(40, m.width-4)
	m.mainViewport.Height = bodyHeight
	m.jobViewport.Width = maxInt(40, m.width-4)
	m.jobViewport.Height = bodyHeight
}

func (m *model) syncDetailViewport() {
	task := m.selectedTask()
	if task == nil {
		m.detailViewport.SetContent("No task selected.")
		return
	}

	lines, err := tasker.TaskStatusDetailsStyled(m.root, task.Meta.ID, tasker.StatusFormatOptions{})
	if err != nil {
		m.detailViewport.SetContent("Error loading task detail: " + err.Error())
		return
	}
	m.detailViewport.SetContent(strings.Join(lines, "\n"))
}

func (m *model) syncMainViewport() {
	switch m.tab {
	case tabDashboard:
		m.mainViewport.SetContent(m.dashboardContent())
	case tabCurrent:
		m.mainViewport.SetContent(m.currentContent())
	case tabJobs:
		if m.activeJob != nil {
			m.jobViewport.SetContent(m.activeJob.Buffer.String())
		}
	}
}

func (m model) renderHeader() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	tabStyle := lipgloss.NewStyle().Padding(0, 1)
	activeTab := tabStyle.Copy().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))

	tabs := []string{"Dashboard", "Tasks", "Current", "Jobs"}
	rendered := make([]string, 0, len(tabs))
	for i, label := range tabs {
		if i == int(m.tab) {
			rendered = append(rendered, activeTab.Render(label))
		} else {
			rendered = append(rendered, tabStyle.Render(label))
		}
	}

	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
		fmt.Sprintf("%s  version %s", m.root, buildinfo.Version),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Tasker TUI"),
		lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(rendered, " "), "  ", meta),
	)
}

func (m model) renderBody() string {
	switch m.tab {
	case tabDashboard:
		return m.panel("Dashboard", m.mainViewport.View())
	case tabCurrent:
		return m.panel("Current Workspace", m.mainViewport.View())
	case tabJobs:
		return m.panel("Jobs", m.jobsContent())
	default:
		return m.renderTasksBody()
	}
}

func (m model) renderTasksBody() string {
	leftWidth := maxInt(24, m.width/2-2)
	rightWidth := maxInt(30, m.width-leftWidth-5)

	left := m.panelWithWidth("Tasks", leftWidth, m.tasksListContent())
	right := m.panelWithWidth("Task Detail", rightWidth, m.detailViewport.View())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

func (m model) renderFooter() string {
	status := m.lastStatus
	if status == "" {
		status = "Ready"
	}
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	footer := []string{
		keyStyle.Render("tab shift+tab switch views"),
		keyStyle.Render("/ filter"),
		keyStyle.Render("S status"),
		keyStyle.Render("T type"),
		keyStyle.Render("n/a/m/c/u/I/d/e/x/s/f actions"),
		keyStyle.Render("? help"),
		keyStyle.Render("q quit"),
	}

	line := statusStyle.Render(status)
	if m.lastErr != "" {
		line = errStyle.Render(m.lastErr)
	}
	return lipgloss.JoinVertical(lipgloss.Left, line, strings.Join(footer, "  "))
}

func (m model) panel(title, body string) string {
	return m.panelWithWidth(title, maxInt(40, m.width-2), body)
}

func (m model) panelWithWidth(title string, width int, body string) string {
	style := lipgloss.NewStyle().
		Width(width).
		Height(maxInt(6, m.height-5)).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))
	return style.Render(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n" + body)
}

func (m model) tasksListContent() string {
	lines := []string{
		fmt.Sprintf("Filter: %s", m.filterInput.View()),
		fmt.Sprintf("Status: %s  Type: %s", m.statusFilter, m.typeFilter),
		"",
	}
	if len(m.filtered) == 0 {
		lines = append(lines, "No matching tasks.")
		return strings.Join(lines, "\n")
	}
	for _, item := range m.filtered {
		prefix := "  "
		if item.Task.Meta.ID == m.selectedTaskID {
			prefix = "> "
		}
		indent := strings.Repeat("  ", item.Depth)
		lines = append(lines, fmt.Sprintf(
			"%s%s%s [%s] (%s)",
			prefix,
			indent,
			tasker.TaskSummary(item.Task),
			item.Task.Meta.Type,
			item.Task.Status.Status,
		))
	}
	return strings.Join(lines, "\n")
}

func (m model) dashboardContent() string {
	if m.snapshot == nil {
		return "Loading workspace snapshot..."
	}

	lines := []string{
		"Current task:",
	}
	if m.snapshot.Current.Task == nil {
		lines = append(lines, "- none")
	} else {
		lines = append(lines,
			fmt.Sprintf("- %s", tasker.TaskSummary(*m.snapshot.Current.Task)),
			fmt.Sprintf("- status: %s", m.snapshot.Current.Task.Status.Status),
			fmt.Sprintf("- type: %s", m.snapshot.Current.Task.Meta.Type),
		)
		if m.snapshot.Current.Branch != "" {
			lines = append(lines, fmt.Sprintf("- branch: %s", m.snapshot.Current.Branch))
		}
	}

	lines = append(lines, "", "Status counts:")
	statuses := tasker.ValidTaskStatuses()
	slices.Sort(statuses)
	for _, status := range statuses {
		lines = append(lines, fmt.Sprintf("- %s: %d", status, m.snapshot.StatusCounts[status]))
	}

	lines = append(lines, "", "Actionable queue:")
	actionable := []string{"NEW", "RUNNING", "BLOCKED", "HANDOFF", "REVIEW", "AWAITING_ACTION"}
	found := 0
	for _, item := range m.snapshot.Tree {
		if !slices.Contains(actionable, item.Task.Status.Status) {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] (%s)", tasker.TaskSummary(item.Task), item.Task.Status.Status, item.Task.Meta.Type))
		found++
		if found >= 12 {
			break
		}
	}
	if found == 0 {
		lines = append(lines, "- none")
	}
	return strings.Join(lines, "\n")
}

func (m model) currentContent() string {
	if m.snapshot == nil {
		return "Loading workspace snapshot..."
	}
	if m.snapshot.Current.Task == nil {
		return "No current task is checked out."
	}

	task := m.snapshot.Current.Task
	lines := []string{
		fmt.Sprintf("Current: %s", tasker.TaskSummary(*task)),
		fmt.Sprintf("Path: %s", task.Path),
		fmt.Sprintf("Status: %s", task.Status.Status),
	}
	if m.snapshot.Current.Branch != "" {
		lines = append(lines, fmt.Sprintf("Git branch: %s", m.snapshot.Current.Branch))
	}

	lines = append(lines, "", "Parent chain:")
	if len(m.snapshot.Current.ParentChain) == 0 {
		lines = append(lines, "- none")
	} else {
		for _, parent := range m.snapshot.Current.ParentChain {
			lines = append(lines, fmt.Sprintf("- %s", tasker.TaskSummary(parent)))
		}
	}

	lines = append(lines, "", "Workspace files:")
	for _, path := range []string{
		filepath.Join(m.root, ".tasker", "current", "WORKSPACE.md"),
		filepath.Join(m.root, ".tasker", "current", "FILES.md"),
		filepath.Join(m.root, ".tasker", "current", "CONTEXT.json"),
	} {
		lines = append(lines, "- "+path)
	}

	lines = append(lines, "", "Task files:")
	for _, path := range []string{
		task.TaskFile,
		task.InstructionsFile,
		task.DeclarationFile,
		task.ResultFile,
		task.MetaFile,
	} {
		lines = append(lines, "- "+path)
	}
	return strings.Join(lines, "\n")
}

func (m model) jobsContent() string {
	if m.activeJob == nil {
		return "No job has been started yet."
	}
	header := []string{
		fmt.Sprintf("Job: %s", m.activeJob.Label),
		fmt.Sprintf("Started: %s", m.activeJob.Started.Format(time.RFC3339)),
	}
	if m.activeJob.Running {
		header = append(header, "Status: running")
	} else {
		header = append(header, "Status: finished")
	}
	content := strings.Join(header, "\n") + "\n\n" + m.jobViewport.View()
	return content
}

func (m model) renderHelp() string {
	lines := []string{
		"Global",
		"tab / shift+tab switch views",
		"/ focuses the task filter",
		"S cycles status filters, T cycles type filters",
		"r refreshes data, ? toggles help, q quits",
		"",
		"Tasks",
		"up/down move selection",
		"n new task, a add child, m edit meta, c checkout",
		"u import tasks, I create import template, d delete",
		"e open task/project documents in the configured editor",
		"x run `tasker do`, s resume session, f fork session",
		"enter opens the Current view for the selected task",
		"",
		"Forms",
		"tab moves between fields",
		"left/right cycle select fields",
		"ctrl+s submits, esc cancels",
		"",
		"Delete confirmation",
		"left/right toggle confirm",
		"r toggles recursive delete",
	}
	return centeredBox("Help", strings.Join(lines, "\n"), minInt(88, m.width-6))
}

func (m model) renderForm() string {
	lines := make([]string, 0, len(m.form.Fields)*3+4)
	for i := range m.form.Fields {
		field := m.form.Fields[i]
		marker := "  "
		if i == m.form.Focus {
			marker = "> "
		}
		value := field.Value()
		if field.Kind == fieldText {
			value = field.Input.View()
		}
		lines = append(lines, marker+field.Label+": "+value)
		if field.Help != "" {
			lines = append(lines, "    "+field.Help)
		}
		lines = append(lines, "")
	}
	lines = append(lines, "ctrl+s submit  esc cancel")
	return centeredBox(m.form.Title, strings.Join(lines, "\n"), minInt(92, m.width-6))
}

func (m model) renderSessionPicker() string {
	lines := []string{
		fmt.Sprintf("Choose a session to %s:", m.session.Action),
		"",
	}
	for i, session := range m.session.Sessions {
		marker := "  "
		if i == m.session.Index {
			marker = "> "
		}
		command := session.ResumeCommand
		if m.session.Action == tasker.AgentSessionFork {
			command = session.ForkCommand
		}
		lines = append(lines, marker+session.Agent+" "+session.ID)
		lines = append(lines, "    "+command)
	}
	lines = append(lines, "", "enter run  esc cancel")
	return centeredBox("Stored Sessions", strings.Join(lines, "\n"), minInt(96, m.width-6))
}

func (m model) renderConfirm() string {
	choiceNo := "[No]"
	choiceYes := "[Yes]"
	if m.confirm.SelectedYes {
		choiceYes = "[YES]"
	} else {
		choiceNo = "[NO]"
	}
	lines := []string{
		m.confirm.Body,
		"",
		fmt.Sprintf("Recursive delete: %t  (press r to toggle)", m.confirm.Recursive),
		"",
		choiceNo + "   " + choiceYes,
	}
	return centeredBox(m.confirm.Title, strings.Join(lines, "\n"), minInt(72, m.width-6))
}

func (m *model) newTaskForm() *formModal {
	form := &formModal{
		Kind:        modalNewTask,
		Title:       "New Task",
		SubmitLabel: "Create",
		Fields: []formField{
			newTextField("title", "Title", "", "Untitled task", "Defaults to `Untitled task` if left blank."),
			newSelectField("type", "Type", tasker.ValidTaskTypes(), "feature", ""),
			newSelectField("checkout_mode", "Checkout", []string{"none", "checkout", "branch-checkout"}, "none", "Optional current-workspace checkout after creation."),
			newSelectField("open_behavior", "Open editor", []string{"no-open", "open"}, "no-open", ""),
			newSelectField("open_target", "Open target", []string{"task", "instructions", "declaration", "result", "meta"}, "task", ""),
		},
	}
	form.FocusCurrent()
	return form
}

func (m *model) newChildTaskForm() *formModal {
	parentID := ""
	if task := m.selectedTask(); task != nil {
		parentID = task.Meta.ID
	} else if m.snapshot != nil && m.snapshot.Current.Task != nil {
		parentID = m.snapshot.Current.Task.Meta.ID
	}
	form := &formModal{
		Kind:        modalAddTask,
		Title:       "Add Child Task",
		SubmitLabel: "Create",
		Fields: []formField{
			newTextField("title", "Title", "", "Untitled task", ""),
			newSelectField("type", "Type", tasker.ValidTaskTypes(), "feature", ""),
			newTextField("parent_id", "Parent ID", parentID, "029", "Leave empty only if you want inference to fail visibly."),
			newSelectField("open_behavior", "Open editor", []string{"no-open", "open"}, "no-open", ""),
			newSelectField("open_target", "Open target", []string{"task", "instructions", "declaration", "result", "meta"}, "task", ""),
		},
	}
	form.FocusCurrent()
	return form
}

func (m *model) newMetaForm(task tasker.Task) *formModal {
	form := &formModal{
		Kind:        modalMeta,
		Title:       "Edit Metadata",
		SubmitLabel: "Save",
		Fields: []formField{
			newTextField("title", "Title", task.Meta.Title, "", ""),
			newSelectField("type", "Type", tasker.ValidTaskTypes(), task.Meta.Type, ""),
			newSelectField("open_behavior", "Open meta after save", []string{"no-open", "open"}, "no-open", ""),
		},
	}
	form.FocusCurrent()
	return form
}

func (m *model) newCheckoutForm(task tasker.Task) *formModal {
	form := &formModal{
		Kind:        modalCheckout,
		Title:       "Checkout Task " + task.Meta.ID,
		SubmitLabel: "Checkout",
		Fields: []formField{
			newSelectField("mode", "Mode", []string{"auto", "no-branch", "custom-branch", "existing-branch"}, "auto", "Matches CLI checkout modes."),
			newTextField("branch", "Branch", "", "task/029-add-tasker-tui-application", "Used for custom or existing branch modes."),
		},
	}
	form.FocusCurrent()
	return form
}

func (m *model) newImportForm() *formModal {
	latest, _ := tasker.LatestImportPath(m.root)
	parentID := ""
	if task := m.selectedTask(); task != nil {
		parentID = task.Meta.ID
	}
	form := &formModal{
		Kind:        modalImport,
		Title:       "Import Tasks",
		SubmitLabel: "Import",
		Fields: []formField{
			newTextField("path", "Import path", latest, ".tasker/imports/import-*.json", "Leave blank to use the most recent import file."),
			newTextField("parent_id", "Parent ID", parentID, "", "Optional parent attachment for imported roots."),
			newSelectField("checkout_mode", "Checkout", []string{"none", "checkout", "branch-checkout"}, "none", ""),
			newSelectField("open_behavior", "Open editor", []string{"no-open", "open"}, "no-open", ""),
			newSelectField("open_target", "Open target", []string{"task", "instructions", "declaration", "result", "meta"}, "task", ""),
		},
	}
	form.FocusCurrent()
	return form
}

func (m *model) newOpenDocForm() *formModal {
	form := &formModal{
		Kind:        modalOpenDoc,
		Title:       "Open Document",
		SubmitLabel: "Open",
		Fields: []formField{
			newSelectField("target", "Target", []string{"task", "instructions", "declaration", "result", "meta", projectInstructionsTarget}, "task", ""),
		},
	}
	form.FocusCurrent()
	return form
}

func refreshCmd(root string) tea.Cmd {
	return func() tea.Msg {
		snapshot, err := tasker.LoadWorkspaceSnapshot(root)
		return snapshotMsg{
			Snapshot: snapshot,
			Err:      err,
		}
	}
}

func pollJobCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return jobTickMsg{}
	})
}

func mutationCmd(status, selectedID string, refresh bool, fn func() (*exec.Cmd, error)) tea.Cmd {
	return func() tea.Msg {
		cmd, err := fn()
		return mutationResultMsg{
			Status:     status,
			SelectedID: selectedID,
			Exec:       cmd,
			Err:        err,
			Refresh:    refresh,
		}
	}
}

func errorCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return mutationResultMsg{Err: err}
	}
}

func (m *model) withError(message string) *model {
	m.lastErr = message
	return m
}

func cycleOption(current string, options []string) string {
	if len(options) == 0 {
		return current
	}
	index := 0
	for i, option := range options {
		if option == current {
			index = i
			break
		}
	}
	return options[(index+1)%len(options)]
}

func centeredBox(title, body string, width int) string {
	style := lipgloss.NewStyle().
		Width(width).
		Padding(1).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("62"))
	return style.Render(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n" + body)
}

func overlay(base, modal string) string {
	return lipgloss.Place(
		lipgloss.Width(base),
		lipgloss.Height(base),
		lipgloss.Center,
		lipgloss.Center,
		modal,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("236")),
	)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
