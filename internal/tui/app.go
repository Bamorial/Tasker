package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bamorial/tasker/internal/buildinfo"
	"github.com/bamorial/tasker/internal/tasker"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type panelFocus int

const (
	panelCurrent panelFocus = iota
	panelTasks
	panelWorkers
)

type currentViewMode string

const (
	viewAuto   currentViewMode = "auto"
	viewTask   currentViewMode = "task"
	viewResult currentViewMode = "result"
	viewStatus currentViewMode = "status"
	viewAgent  currentViewMode = "agent"
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

type jobState struct {
	Label   string
	TaskID  string
	Task    tasker.Task
	Started time.Time
	Running bool
}

type visibleTaskItem struct {
	Task       tasker.Task
	Depth      int
	ChildCount int
	HasChild   bool
	Expanded   bool
}

type workerTaskItem struct {
	Task   tasker.Task
	Active bool
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

type workspaceTickMsg struct{}

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

	focus           panelFocus
	currentViewMode currentViewMode
	help            bool

	snapshot *tasker.WorkspaceSnapshot

	filterInput          textinput.Model
	filtering            bool
	statusFilter         string
	typeFilter           string
	filtered             []visibleTaskItem
	selectedTaskID       string
	selectedWorkerTaskID string
	expandedTasks        map[string]bool

	taskViewport    viewport.Model
	currentViewport viewport.Model
	workerViewport  viewport.Model

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

	taskVP := viewport.New(0, 0)
	currentVP := viewport.New(0, 0)
	workerVP := viewport.New(0, 0)

	return model{
		root:            root,
		focus:           panelTasks,
		currentViewMode: viewAuto,
		filterInput:     filter,
		statusFilter:    filterAll,
		typeFilter:      filterAll,
		expandedTasks:   make(map[string]bool),
		taskViewport:    taskVP,
		currentViewport: currentVP,
		workerViewport:  workerVP,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.root), workspaceTickCmd())
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
		m.reconcileActiveJob()
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
	case workspaceTickMsg:
		if m.currentViewMode == viewAgent {
			m.syncCurrentViewport()
		}
		if m.hasRunningTasks() {
			return m, tea.Batch(refreshCmd(m.root), workspaceTickCmd())
		}
		return m, workspaceTickCmd()
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
	switch m.focus {
	case panelTasks:
		m.taskViewport, cmd = m.taskViewport.Update(msg)
	case panelCurrent:
		m.currentViewport, cmd = m.currentViewport.Update(msg)
	case panelWorkers:
		m.workerViewport, cmd = m.workerViewport.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
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
	case "0":
		m.focus = panelCurrent
		return m, nil
	case "1":
		m.focus = panelTasks
		return m, nil
	case "2":
		m.focus = panelWorkers
		return m, nil
	case "?":
		m.help = !m.help
		return m, nil
	case "R":
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

	switch m.focus {
	case panelTasks:
		return m.updateTasksViewKeys(msg)
	case panelCurrent:
		return m.updateCurrentViewKeys(msg)
	case panelWorkers:
		return m.updateWorkersViewKeys(msg)
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
		m.toggleSelectedTaskExpansion()
		m.focus = panelCurrent
		m.syncDerivedState()
		return m, nil
	default:
		return m.updateViewportKeys(msg, &m.taskViewport)
	}
}

func (m *model) updateCurrentViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "t":
		m.currentViewMode = viewTask
		m.syncCurrentViewport()
		return m, nil
	case "r":
		m.currentViewMode = viewResult
		m.syncCurrentViewport()
		return m, nil
	case "s":
		m.currentViewMode = viewStatus
		m.syncCurrentViewport()
		return m, nil
	case "w":
		m.currentViewMode = viewAgent
		m.syncCurrentViewport()
		return m, nil
	case "e":
		return m.openSelectedTaskInEditor()
	case "x":
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		return m.startDoJob(*task)
	case "S":
		return m.openSessionPicker(tasker.AgentSessionResume)
	case "F":
		return m.openSessionPicker(tasker.AgentSessionFork)
	default:
		return m.updateViewportKeys(msg, &m.currentViewport)
	}
}

func (m *model) updateWorkersViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.moveWorkerSelection(-1)
		return m, nil
	case "down", "j":
		m.moveWorkerSelection(1)
		return m, nil
	case "pgup":
		m.moveWorkerSelection(-5)
		return m, nil
	case "pgdown":
		m.moveWorkerSelection(5)
		return m, nil
	case "enter":
		task := m.selectedWorkerTask()
		if task == nil {
			return m.withError("no running task selected"), nil
		}
		m.selectedTaskID = task.Meta.ID
		m.currentViewMode = viewAgent
		m.focus = panelCurrent
		m.syncDerivedState()
		return m, nil
	default:
		return m.updateViewportKeys(msg, &m.workerViewport)
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
	m.activeJob = &jobState{
		Label:   "tasker do " + task.Meta.ID,
		TaskID:  task.Meta.ID,
		Task:    task,
		Started: time.Now(),
		Running: true,
	}
	m.selectedWorkerTaskID = task.Meta.ID
	m.focus = panelWorkers
	m.syncWorkerViewport()
	return m, mutationCmd("Started tasker do for "+task.Meta.ID, task.Meta.ID, true, func() (*exec.Cmd, error) {
		return nil, tasker.StartDetachedDoTask(m.root, task.Meta.ID)
	})
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
	m.filtered = m.buildVisibleTasks(query)
	m.ensureSelection()
}

func (m *model) ensureSelection() {
	if len(m.filtered) == 0 {
		m.selectedTaskID = ""
		return
	}
	if m.selectedTaskID != "" {
		m.expandTaskPath(m.selectedTaskID)
		for _, item := range m.filtered {
			if item.Task.Meta.ID == m.selectedTaskID {
				return
			}
		}
		m.filtered = m.buildVisibleTasks(strings.ToLower(strings.TrimSpace(m.filterInput.Value())))
	}
	for _, item := range m.filtered {
		if item.Task.Meta.ID == m.selectedTaskID {
			return
		}
	}

	if m.snapshot != nil && m.snapshot.Current.Task != nil {
		m.expandTaskPath(m.snapshot.Current.Task.Meta.ID)
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
	m.ensureWorkerSelection()
	m.resizeViewports()
	m.syncTaskViewport()
	m.syncCurrentViewport()
	m.syncWorkerViewport()
}

func (m *model) resizeViewports() {
	bodyHeight := maxInt(12, m.height-6)
	leftWidth := maxInt(30, m.width/3)
	rightWidth := maxInt(40, m.width-leftWidth-3)
	taskPanelHeight := maxInt(10, (bodyHeight*2)/3)
	workerPanelHeight := maxInt(8, bodyHeight-taskPanelHeight)
	if taskPanelHeight+workerPanelHeight > bodyHeight {
		taskPanelHeight = bodyHeight - workerPanelHeight
	}

	m.taskViewport.Width = maxInt(12, leftWidth-4)
	m.taskViewport.Height = maxInt(3, taskPanelHeight-4)
	m.currentViewport.Width = maxInt(16, rightWidth-4)
	m.currentViewport.Height = maxInt(3, bodyHeight-4)
	m.workerViewport.Width = maxInt(12, leftWidth-4)
	m.workerViewport.Height = maxInt(3, workerPanelHeight-4)
}

func (m model) renderHeader() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	meta := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(
		fmt.Sprintf("%s  version %s", m.root, buildinfo.Version),
	)
	focus := metaStyle.Render("focus 0=current 1=tasks 2=workers")
	return lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("Tasker TUI"),
		lipgloss.JoinHorizontal(lipgloss.Top, focus, "  ", meta),
	)
}

func (m model) renderBody() string {
	bodyHeight := maxInt(12, m.height-6)
	leftWidth := maxInt(30, m.width/3)
	rightWidth := maxInt(40, m.width-leftWidth-3)
	taskPanelHeight := maxInt(10, (bodyHeight*2)/3)
	workerPanelHeight := maxInt(8, bodyHeight-taskPanelHeight)
	if taskPanelHeight+workerPanelHeight > bodyHeight {
		taskPanelHeight = bodyHeight - workerPanelHeight
	}

	left := lipgloss.JoinVertical(
		lipgloss.Left,
		m.panelWithDimensions("1 Tasks", leftWidth, taskPanelHeight, m.focus == panelTasks, m.taskViewport.View()),
		m.panelWithDimensions("2 Workers", leftWidth, workerPanelHeight, m.focus == panelWorkers, m.workerViewport.View()),
	)
	right := m.panelWithDimensions("0 Current View", rightWidth, bodyHeight, m.focus == panelCurrent, m.currentViewport.View())
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
		keyStyle.Render("0/1/2 focus panels"),
		keyStyle.Render("/ filter"),
		keyStyle.Render("S status filter"),
		keyStyle.Render("T type"),
		keyStyle.Render("enter open task/output"),
		keyStyle.Render("t/r/s/w switch current view"),
		keyStyle.Render("e edit  x do  S resume  F fork"),
		keyStyle.Render("R refresh"),
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
	return m.panelWithDimensions(title, maxInt(40, m.width-2), maxInt(6, m.height-5), false, body)
}

func (m model) panelWithDimensions(title string, width, height int, focused bool, body string) string {
	border := lipgloss.Color("240")
	if focused {
		border = lipgloss.Color("62")
	}
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border)
	titleStyle := lipgloss.NewStyle().Bold(true)
	return style.Render(titleStyle.Render(title) + "\n" + body)
}

func (m *model) syncTaskViewport() {
	m.taskViewport.SetContent(m.tasksListContent())
	selectedLine := 2
	for i, item := range m.filtered {
		if item.Task.Meta.ID == m.selectedTaskID {
			selectedLine += i
			break
		}
	}
	ensureViewportContains(&m.taskViewport, selectedLine)
}

func (m model) tasksListContent() string {
	lines := []string{
		fmt.Sprintf("Filter: %s", m.filterInput.View()),
		fmt.Sprintf("Status: %s  Type: %s", m.statusFilter, m.typeFilter),
	}
	if len(m.filtered) == 0 {
		lines = append(lines, "", "No matching tasks.")
		return strings.Join(lines, "\n")
	}
	for _, item := range m.filtered {
		prefix := "  "
		if item.Task.Meta.ID == m.selectedTaskID {
			prefix = "> "
		}
		currentMarker := " "
		if m.snapshot != nil && m.snapshot.Current.Task != nil && m.snapshot.Current.Task.Meta.ID == item.Task.Meta.ID {
			currentMarker = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")).Render("*")
		}
		indent := strings.Repeat("  ", item.Depth)
		marker := "•"
		if item.HasChild {
			if item.Expanded {
				marker = "▾"
			} else {
				marker = "▸"
			}
		}
		line := fmt.Sprintf("%s%s%s %s %s %s %s",
			prefix,
			currentMarker,
			indent,
			marker,
			item.Task.Meta.ID,
			renderTaskStatusBadge(item.Task.Status.Status),
			item.Task.Meta.Title,
		)
		lines = append(lines, truncateTaskListLine(line, m.taskViewport.Width))
	}
	return strings.Join(lines, "\n")
}

func (m *model) syncCurrentViewport() {
	m.currentViewport.SetContent(m.currentContent())
	m.currentViewport.GotoTop()
}

func (m model) currentContent() string {
	task := m.selectedTask()
	if task == nil {
		return "No task selected."
	}

	mode := m.currentViewMode
	if mode == viewAuto {
		if content, err := readTaskFile(task.ResultFile); err == nil && strings.TrimSpace(content) != "# Result\n\nSummary:" {
			return m.decorateCurrentContent(*task, "result.md", content)
		}
		mode = viewTask
	}

	switch mode {
	case viewResult:
		content, err := readTaskFile(task.ResultFile)
		if err != nil {
			return "Error loading result.md: " + err.Error()
		}
		return m.decorateCurrentContent(*task, "result.md", content)
	case viewStatus:
		lines, err := tasker.TaskStatusDetailsStyled(m.root, task.Meta.ID, tasker.StatusFormatOptions{})
		if err != nil {
			return "Error loading status: " + err.Error()
		}
		return m.decorateCurrentContent(*task, "status", strings.Join(lines, "\n"))
	case viewAgent:
		return m.decorateCurrentContent(*task, "agent output", m.agentOutputContent(*task))
	default:
		content, err := readTaskFile(task.TaskFile)
		if err != nil {
			return "Error loading task.md: " + err.Error()
		}
		return m.decorateCurrentContent(*task, "task.md", content)
	}
}

func (m model) decorateCurrentContent(task tasker.Task, label, content string) string {
	lines := []string{
		fmt.Sprintf("Task: %s", tasker.TaskSummary(task)),
		fmt.Sprintf("View: %s", label),
		fmt.Sprintf("Status: %s", task.Status.Status),
		fmt.Sprintf("Type: %s", task.Meta.Type),
		"",
		content,
	}
	return strings.Join(lines, "\n")
}

func (m *model) syncWorkerViewport() {
	m.workerViewport.SetContent(m.workerContent())
	selectedLine := 1
	for i, item := range m.workerTasks() {
		if item.Task.Meta.ID == m.selectedWorkerTaskID {
			selectedLine += i
			break
		}
	}
	ensureViewportContains(&m.workerViewport, selectedLine)
}

func (m model) workerContent() string {
	lines := []string{"Running tasks:"}
	items := m.workerTasks()
	if len(items) == 0 {
		lines = append(lines, "- none")
		return strings.Join(lines, "\n")
	}

	for _, item := range items {
		prefix := "  "
		if item.Task.Meta.ID == m.selectedWorkerTaskID {
			prefix = "> "
		}
		line := fmt.Sprintf("%s%s %s %s",
			prefix,
			item.Task.Meta.ID,
			renderTaskStatusBadge(item.Task.Status.Status),
			item.Task.Meta.Title,
		)
		if item.Active {
			line += " (live)"
		}
		lines = append(lines, truncateTaskListLine(line, m.workerViewport.Width))
	}
	return strings.Join(lines, "\n")
}

func (m model) workerTasks() []workerTaskItem {
	items := make([]workerTaskItem, 0)
	seen := make(map[string]struct{})
	if m.snapshot != nil {
		for _, item := range m.snapshot.Tree {
			if item.Task.Status.Status != "RUNNING" {
				continue
			}
			if _, ok := seen[item.Task.Meta.ID]; ok {
				continue
			}
			seen[item.Task.Meta.ID] = struct{}{}
			items = append(items, workerTaskItem{
				Task:   item.Task,
				Active: m.activeJob != nil && m.activeJob.TaskID == item.Task.Meta.ID,
			})
		}
	}
	if m.activeJob != nil && m.activeJob.Running {
		if _, ok := seen[m.activeJob.TaskID]; !ok {
			items = append(items, workerTaskItem{
				Task:   m.activeJob.Task,
				Active: true,
			})
		}
	}
	return items
}

func (m *model) ensureWorkerSelection() {
	items := m.workerTasks()
	if len(items) == 0 {
		m.selectedWorkerTaskID = ""
		return
	}
	for _, item := range items {
		if item.Task.Meta.ID == m.selectedWorkerTaskID {
			return
		}
	}
	if m.activeJob != nil {
		for _, item := range items {
			if item.Task.Meta.ID == m.activeJob.TaskID {
				m.selectedWorkerTaskID = item.Task.Meta.ID
				return
			}
		}
	}
	m.selectedWorkerTaskID = items[0].Task.Meta.ID
}

func (m *model) moveWorkerSelection(delta int) {
	items := m.workerTasks()
	if len(items) == 0 {
		m.selectedWorkerTaskID = ""
		return
	}

	index := 0
	for i, item := range items {
		if item.Task.Meta.ID == m.selectedWorkerTaskID {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = 0
	}
	if index >= len(items) {
		index = len(items) - 1
	}
	m.selectedWorkerTaskID = items[index].Task.Meta.ID
	m.syncWorkerViewport()
}

func (m *model) selectedWorkerTask() *tasker.Task {
	for _, item := range m.workerTasks() {
		if item.Task.Meta.ID == m.selectedWorkerTaskID {
			task := item.Task
			return &task
		}
	}
	return nil
}

func (m model) agentOutputContent(task tasker.Task) string {
	lines := make([]string, 0, 8)
	session, output, err := tasker.ReadTaskAgentOutput(&task)
	if err == nil {
		lines = append(lines,
			fmt.Sprintf("Source: %s session %s", session.Agent, session.ID),
		)
		if session.RecordedAt != "" {
			lines = append(lines, "Recorded: "+session.RecordedAt)
		}
		lines = append(lines, "", output)
		return strings.Join(lines, "\n")
	}

	if m.activeJob != nil && m.activeJob.TaskID == task.Meta.ID && m.activeJob.Running {
		return strings.Join([]string{
			"Waiting for Codex session output...",
			"Started: " + m.activeJob.Started.Format(time.RFC3339),
		}, "\n")
	}

	if len(task.Status.Sessions) == 0 {
		return "No stored agent session was found for this task."
	}
	return "No readable agent output was found for this task yet."
}

func (m *model) reconcileActiveJob() {
	if m.activeJob == nil || m.snapshot == nil {
		return
	}
	for _, task := range m.snapshot.Tasks {
		if task.Meta.ID != m.activeJob.TaskID {
			continue
		}
		if task.Status.Status != "RUNNING" {
			m.activeJob = nil
			return
		}
		m.activeJob.Task = task
		return
	}
	m.activeJob = nil
}

func (m model) hasRunningTasks() bool {
	if m.activeJob != nil && m.activeJob.Running {
		return true
	}
	if m.snapshot == nil {
		return false
	}
	for _, task := range m.snapshot.Tasks {
		if task.Status.Status == "RUNNING" {
			return true
		}
	}
	return false
}

func (m *model) buildVisibleTasks(query string) []visibleTaskItem {
	if m.snapshot == nil {
		return nil
	}

	byParent := make(map[string][]tasker.Task)
	for _, task := range m.snapshot.Tasks {
		byParent[task.Meta.ParentID] = append(byParent[task.Meta.ParentID], task)
	}
	for parentID := range byParent {
		slices.SortFunc(byParent[parentID], func(a, b tasker.Task) int {
			return strings.Compare(a.Meta.ID, b.Meta.ID)
		})
	}

	childCountMemo := make(map[string]int)
	var childCount func(string) int
	childCount = func(id string) int {
		if count, ok := childCountMemo[id]; ok {
			return count
		}
		total := 0
		for _, child := range byParent[id] {
			total++
			total += childCount(child.Meta.ID)
		}
		childCountMemo[id] = total
		return total
	}

	matches := func(task tasker.Task) bool {
		if m.statusFilter != filterAll && task.Status.Status != m.statusFilter {
			return false
		}
		if m.typeFilter != filterAll && task.Meta.Type != m.typeFilter {
			return false
		}
		if query == "" {
			return true
		}
		haystack := strings.ToLower(strings.Join([]string{
			task.Meta.ID,
			task.Meta.Title,
			task.Meta.Slug,
			task.Status.Status,
			task.Meta.Type,
		}, " "))
		return strings.Contains(haystack, query)
	}

	filtering := query != "" || m.statusFilter != filterAll || m.typeFilter != filterAll
	items := make([]visibleTaskItem, 0, len(m.snapshot.Tasks))
	var walk func(parentID string, depth int)
	walk = func(parentID string, depth int) {
		for _, task := range byParent[parentID] {
			children := byParent[task.Meta.ID]
			item := visibleTaskItem{
				Task:       task,
				Depth:      depth,
				ChildCount: childCount(task.Meta.ID),
				HasChild:   len(children) > 0,
				Expanded:   m.expandedTasks[task.Meta.ID],
			}
			if matches(task) {
				items = append(items, item)
			}
			if filtering || item.Expanded {
				walk(task.Meta.ID, depth+1)
			}
		}
	}
	walk("", 0)
	return items
}

func (m *model) toggleSelectedTaskExpansion() {
	task := m.selectedTask()
	if task == nil {
		return
	}
	for _, item := range m.filtered {
		if item.Task.Meta.ID == task.Meta.ID && item.HasChild {
			m.expandedTasks[task.Meta.ID] = !m.expandedTasks[task.Meta.ID]
			return
		}
	}
}

func (m *model) expandTaskPath(id string) {
	if m.snapshot == nil {
		return
	}
	task, err := tasker.GetTask(m.root, id)
	if err != nil {
		return
	}
	for task != nil && task.Meta.ParentID != "" {
		m.expandedTasks[task.Meta.ParentID] = true
		parent, err := tasker.GetTask(m.root, task.Meta.ParentID)
		if err != nil {
			break
		}
		task = parent
	}
}

func (m *model) openSelectedTaskInEditor() (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m.withError("select a task first"), nil
	}

	target := task.TaskFile
	status := "Opened " + target
	if m.currentViewMode == viewResult {
		target = task.ResultFile
		status = "Opened " + target
	}

	cmd, err := tasker.EditorCommand(m.root, target)
	if err != nil {
		return m.withError("editor unavailable, file path: " + target), nil
	}

	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return externalDoneMsg{
			Status:     status,
			SelectedID: task.Meta.ID,
			Err:        err,
			Refresh:    false,
		}
	})
}

func (m model) renderHelp() string {
	lines := []string{
		"Global",
		"0/1/2 focus current, tasks, and workers panels",
		"/ focuses the task filter",
		"S cycles status filters, T cycles type filters",
		"R refreshes data, ? toggles help, q quits",
		"",
		"Tasks",
		"up/down move selection",
		"enter toggles subtasks and opens the current-view panel",
		"n new task, a add child, m edit meta, c checkout",
		"u import tasks, I create import template, d delete",
		"e open task/project documents in the configured editor",
		"x run `tasker do`, s resume session, f fork session",
		"",
		"Current View",
		"t shows task.md, r shows result.md, s shows status, w shows agent output",
		"e opens the selected task or result in your editor",
		"x runs `tasker do`, S resumes a stored session, F forks one",
		"",
		"Workers",
		"up/down move between running tasks",
		"enter opens the selected task's agent output",
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
	openBehavior := defaultOpenBehavior(m.root)
	form := &formModal{
		Kind:        modalNewTask,
		Title:       "New Task",
		SubmitLabel: "Create",
		Fields: []formField{
			newTextField("title", "Title", "", "Untitled task", "Defaults to `Untitled task` if left blank."),
			newSelectField("type", "Type", tasker.ValidTaskTypes(), "feature", ""),
			newSelectField("checkout_mode", "Checkout", []string{"none", "checkout", "branch-checkout"}, "none", "Optional current-workspace checkout after creation."),
			newSelectField("open_behavior", "Open editor", []string{"no-open", "open"}, openBehavior, ""),
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
	openBehavior := defaultOpenBehavior(m.root)
	form := &formModal{
		Kind:        modalAddTask,
		Title:       "Add Child Task",
		SubmitLabel: "Create",
		Fields: []formField{
			newTextField("title", "Title", "", "Untitled task", ""),
			newSelectField("type", "Type", tasker.ValidTaskTypes(), "feature", ""),
			newTextField("parent_id", "Parent ID", parentID, "029", "Leave empty only if you want inference to fail visibly."),
			newSelectField("open_behavior", "Open editor", []string{"no-open", "open"}, openBehavior, ""),
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
	openBehavior := defaultOpenBehavior(m.root)
	form := &formModal{
		Kind:        modalImport,
		Title:       "Import Tasks",
		SubmitLabel: "Import",
		Fields: []formField{
			newTextField("path", "Import path", latest, ".tasker/imports/import-*.json", "Leave blank to use the most recent import file."),
			newTextField("parent_id", "Parent ID", parentID, "", "Optional parent attachment for imported roots."),
			newSelectField("checkout_mode", "Checkout", []string{"none", "checkout", "branch-checkout"}, "none", ""),
			newSelectField("open_behavior", "Open editor", []string{"no-open", "open"}, openBehavior, ""),
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

func defaultOpenBehavior(root string) string {
	if editor, err := tasker.ResolveEditor(root); err == nil && editor != "" {
		return "open"
	}
	return "no-open"
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

func workspaceTickCmd() tea.Cmd {
	return tea.Tick(750*time.Millisecond, func(time.Time) tea.Msg {
		return workspaceTickMsg{}
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

func readTaskFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func ensureViewportContains(vp *viewport.Model, line int) {
	if line < vp.YOffset {
		vp.SetYOffset(maxInt(0, line))
		return
	}
	bottom := vp.YOffset + vp.Height - 1
	if line > bottom {
		vp.SetYOffset(maxInt(0, line-vp.Height+1))
	}
}

func truncateTaskListLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(line) <= width {
		return line
	}
	return runewidth.Truncate(line, width, "...")
}

func renderTaskStatusBadge(status string) string {
	badge := "[" + strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(status)), "_", " ") + "]"

	color := lipgloss.Color("51")
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "NEW":
		color = lipgloss.Color("34")
	case "PLANNED":
		color = lipgloss.Color("51")
	case "IN_PROGRESS", "RUNNING":
		color = lipgloss.Color("214")
	case "DONE":
		color = lipgloss.Color("42")
	case "BLOCKED":
		color = lipgloss.Color("196")
	case "HANDOFF", "REVIEW", "AWAITING_ACTION":
		color = lipgloss.Color("170")
	}

	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(badge)
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
