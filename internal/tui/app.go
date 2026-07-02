package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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
	viewDiff   currentViewMode = "diff"
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
	modalStopExecution        modalKind = "stop-execution"
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
	Kind        modalKind
	TaskID      string
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

type fileChangeMsg struct{}

type fileWatchErrorMsg struct {
	Err error
}

type externalDoneMsg struct {
	Status     string
	SelectedID string
	Err        error
	Refresh    bool
}

type model struct {
	root string

	keybindings tasker.TUIKeybindings

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

	currentViewportTaskID string
	currentViewportView   currentViewMode

	form    *formModal
	session *sessionModal
	confirm *confirmModal

	activeJob        *jobState
	lastStatus       string
	lastErr          string
	pendingSelection string
	refreshPending   bool
}

func Run(root string) error {
	m := newModel(root)
	p := tea.NewProgram(m, tea.WithAltScreen())
	watcher, err := startTaskerWatcher(root, p.Send)
	if err != nil {
		return err
	}
	defer watcher.Close()
	_, err = p.Run()
	return err
}

func newModel(root string) model {
	keybindings, configErr := loadKeybindings(root)

	filter := textinput.New()
	filter.Prompt = "Filter: "
	filter.Placeholder = "title, id, slug"
	filter.Width = 24

	taskVP := viewport.New(0, 0)
	currentVP := viewport.New(0, 0)
	workerVP := viewport.New(0, 0)

	return model{
		root:            root,
		keybindings:     keybindings,
		focus:           panelTasks,
		currentViewMode: viewAuto,
		filterInput:     filter,
		statusFilter:    filterAll,
		typeFilter:      filterAll,
		expandedTasks:   make(map[string]bool),
		taskViewport:    taskVP,
		currentViewport: currentVP,
		workerViewport:  workerVP,
		lastErr:         configErr,
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
		m.refreshPending = false
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
	case fileChangeMsg:
		if m.refreshPending {
			return m, nil
		}
		m.refreshPending = true
		return m, refreshCmd(m.root)
	case fileWatchErrorMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
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
			m.syncCurrentViewport(false)
		}
		if !m.refreshPending {
			m.refreshPending = true
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
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Global, "quit", key):
		return m, tea.Quit
	case keyMatches(m.keybindings.Global, "focus_current", key):
		m.focus = panelCurrent
		return m, nil
	case keyMatches(m.keybindings.Global, "focus_tasks", key):
		m.focus = panelTasks
		return m, nil
	case keyMatches(m.keybindings.Global, "focus_workers", key):
		m.focus = panelWorkers
		return m, nil
	case keyMatches(m.keybindings.Global, "toggle_help", key):
		m.help = !m.help
		return m, nil
	case keyMatches(m.keybindings.Global, "refresh", key):
		return m, refreshCmd(m.root)
	case keyMatches(m.keybindings.Global, "filter", key):
		m.filtering = true
		m.filterInput.Focus()
		return m, nil
	case keyMatches(m.keybindings.Global, "cycle_status_filter", key):
		m.statusFilter = cycleOption(m.statusFilter, append([]string{filterAll}, tasker.ValidTaskStatuses()...))
		m.applyFilters()
		return m, nil
	case keyMatches(m.keybindings.Global, "cycle_type_filter", key):
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
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Tasks, "move_up", key):
		m.moveSelection(-1)
		return m, nil
	case keyMatches(m.keybindings.Tasks, "move_down", key):
		m.moveSelection(1)
		return m, nil
	case keyMatches(m.keybindings.Tasks, "page_up", key):
		m.moveSelection(-5)
		return m, nil
	case keyMatches(m.keybindings.Tasks, "page_down", key):
		m.moveSelection(5)
		return m, nil
	case keyMatches(m.keybindings.Tasks, "new_task", key):
		m.form = m.newTaskForm()
		return m, nil
	case keyMatches(m.keybindings.Tasks, "add_child", key):
		m.form = m.newChildTaskForm()
		return m, nil
	case keyMatches(m.keybindings.Tasks, "edit_meta", key):
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.form = m.newMetaForm(*task)
		return m, nil
	case keyMatches(m.keybindings.Current, "show_diff", key):
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.currentViewMode = viewDiff
		m.focus = panelCurrent
		m.syncDerivedState()
		return m, nil
	case keyMatches(m.keybindings.Tasks, "checkout", key):
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.form = m.newCheckoutForm(*task)
		return m, nil
	case keyMatches(m.keybindings.Tasks, "import_tasks", key):
		m.form = m.newImportForm()
		return m, nil
	case keyMatches(m.keybindings.Tasks, "create_import_template", key):
		return m, m.createImportTemplateCmd()
	case keyMatches(m.keybindings.Tasks, "delete_task", key):
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		m.confirm = &confirmModal{
			Kind:      modalDelete,
			TaskID:    task.Meta.ID,
			Title:     "Delete Task",
			Body:      fmt.Sprintf("Delete task %s %s?", task.Meta.ID, task.Meta.Title),
			Recursive: false,
		}
		return m, nil
	case keyMatches(m.keybindings.Tasks, "open_doc", key):
		m.form = m.newOpenDocForm()
		return m, nil
	case keyMatches(m.keybindings.Tasks, "run_do", key):
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		return m.startDoJob(*task)
	case keyMatches(m.keybindings.Tasks, "resume", key):
		return m.resumeSelectedTask(false)
	case keyMatches(m.keybindings.Tasks, "fork_session", key):
		return m.openSessionPicker(tasker.AgentSessionFork)
	case keyMatches(m.keybindings.Tasks, "open_output", key):
		return m.openSelectedTaskAgentOutput()
	case keyMatches(m.keybindings.Tasks, "open_current", key):
		m.toggleSelectedTaskExpansion()
		if task := m.selectedTask(); task != nil {
			if task.Status.Status == "RUNNING" {
				m.currentViewMode = viewAgent
			} else {
				m.currentViewMode = viewAuto
			}
		}
		m.focus = panelCurrent
		m.syncDerivedState()
		return m, nil
	default:
		return m.updateViewportKeys(msg, &m.taskViewport)
	}
}

func (m *model) updateCurrentViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Current, "show_task", key):
		m.currentViewMode = viewTask
		m.syncCurrentViewport(true)
		return m, nil
	case keyMatches(m.keybindings.Current, "show_result", key):
		m.currentViewMode = viewResult
		m.syncCurrentViewport(true)
		return m, nil
	case keyMatches(m.keybindings.Current, "show_status", key):
		m.currentViewMode = viewStatus
		m.syncCurrentViewport(true)
		return m, nil
	case keyMatches(m.keybindings.Current, "show_diff", key):
		m.currentViewMode = viewDiff
		m.syncCurrentViewport(true)
		return m, nil
	case keyMatches(m.keybindings.Current, "show_agent", key):
		m.currentViewMode = viewAgent
		m.syncCurrentViewport(true)
		return m, nil
	case keyMatches(m.keybindings.Current, "open_output", key):
		return m.openSelectedTaskAgentOutput()
	case keyMatches(m.keybindings.Current, "edit_doc", key):
		return m.openSelectedTaskInEditor()
	case keyMatches(m.keybindings.Current, "run_do", key):
		task := m.selectedTask()
		if task == nil {
			return m.withError("select a task first"), nil
		}
		return m.startDoJob(*task)
	case keyMatches(m.keybindings.Current, "resume", key):
		return m.resumeSelectedTask(false)
	case keyMatches(m.keybindings.Current, "fork_session", key):
		return m.openSessionPicker(tasker.AgentSessionFork)
	default:
		return m.updateViewportKeys(msg, &m.currentViewport)
	}
}

func (m *model) updateWorkersViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Workers, "move_up", key):
		m.moveWorkerSelection(-1)
		return m, nil
	case keyMatches(m.keybindings.Workers, "move_down", key):
		m.moveWorkerSelection(1)
		return m, nil
	case keyMatches(m.keybindings.Workers, "page_up", key):
		m.moveWorkerSelection(-5)
		return m, nil
	case keyMatches(m.keybindings.Workers, "page_down", key):
		m.moveWorkerSelection(5)
		return m, nil
	case keyMatches(m.keybindings.Workers, "open_output", key):
		task := m.selectedWorkerTask()
		if task == nil {
			return m.withError("no running task selected"), nil
		}
		m.selectedTaskID = task.Meta.ID
		m.currentViewMode = viewAgent
		m.focus = panelCurrent
		m.syncDerivedState()
		return m, nil
	case keyMatches(m.keybindings.Workers, "stop_task", key):
		task := m.selectedWorkerTask()
		if task == nil {
			return m.withError("no running task selected"), nil
		}
		m.confirm = &confirmModal{
			Kind:   modalStopExecution,
			TaskID: task.Meta.ID,
			Title:  "Stop Task Execution",
			Body:   fmt.Sprintf("Stop execution for task %s %s?", task.Meta.ID, task.Meta.Title),
		}
		return m, nil
	default:
		return m.updateViewportKeys(msg, &m.workerViewport)
	}
}

func (m *model) openSelectedTaskAgentOutput() (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m.withError("select a task first"), nil
	}
	m.currentViewMode = viewAgent
	m.focus = panelCurrent
	m.syncCurrentViewport(true)
	return m, nil
}

func (m *model) updateViewportKeys(msg tea.KeyMsg, vp *viewport.Model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Viewport, "line_up", key):
		vp.LineUp(1)
	case keyMatches(m.keybindings.Viewport, "line_down", key):
		vp.LineDown(1)
	case keyMatches(m.keybindings.Viewport, "page_up", key):
		vp.HalfViewUp()
	case keyMatches(m.keybindings.Viewport, "page_down", key):
		vp.HalfViewDown()
	case keyMatches(m.keybindings.Viewport, "top", key):
		vp.GotoTop()
	case keyMatches(m.keybindings.Viewport, "bottom", key):
		vp.GotoBottom()
	default:
		*vp, cmd = vp.Update(msg)
	}
	return m, cmd
}

func (m *model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Filter, "cancel", key):
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	case keyMatches(m.keybindings.Filter, "apply", key):
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

	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Form, "cancel", key):
		m.form = nil
		return m, nil
	case keyMatches(m.keybindings.Form, "next_field", key):
		m.form.Next()
		return m, nil
	case keyMatches(m.keybindings.Form, "prev_field", key):
		m.form.Prev()
		return m, nil
	case keyMatches(m.keybindings.Form, "prev_option", key):
		if len(m.form.Fields) > 0 {
			m.form.Fields[m.form.Focus].Cycle(-1)
		}
		return m, nil
	case keyMatches(m.keybindings.Form, "next_option", key):
		if len(m.form.Fields) > 0 {
			m.form.Fields[m.form.Focus].Cycle(1)
		}
		return m, nil
	case keyMatches(m.keybindings.Form, "submit", key):
		return m.submitForm()
	case keyMatches(m.keybindings.Form, "submit_or_next", key):
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
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Session, "cancel", key):
		m.session = nil
		return m, nil
	case keyMatches(m.keybindings.Session, "move_up", key):
		if m.session.Index > 0 {
			m.session.Index--
		}
		return m, nil
	case keyMatches(m.keybindings.Session, "move_down", key):
		if m.session.Index < len(m.session.Sessions)-1 {
			m.session.Index++
		}
		return m, nil
	case keyMatches(m.keybindings.Session, "select", key):
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
	key := msg.String()

	switch {
	case keyMatches(m.keybindings.Confirm, "cancel", key):
		m.confirm = nil
		return m, nil
	case keyMatches(m.keybindings.Confirm, "toggle_choice", key):
		m.confirm.SelectedYes = !m.confirm.SelectedYes
		return m, nil
	case keyMatches(m.keybindings.Confirm, "toggle_recursive", key):
		if m.confirm.Kind == modalDelete {
			m.confirm.Recursive = !m.confirm.Recursive
		}
		return m, nil
	case keyMatches(m.keybindings.Confirm, "accept", key):
		if !m.confirm.SelectedYes {
			m.confirm = nil
			return m, nil
		}
		selectedID := m.confirm.TaskID
		kind := m.confirm.Kind
		recursive := m.confirm.Recursive
		m.confirm = nil
		switch kind {
		case modalStopExecution:
			m.selectedTaskID = selectedID
			m.focus = panelTasks
			m.syncDerivedState()
			return m, mutationCmd("Stopped task "+selectedID, selectedID, true, func() (*exec.Cmd, error) {
				return nil, tasker.StopTaskExecution(m.root, selectedID)
			})
		default:
			return m, mutationCmd("Deleted task "+selectedID, selectedID, true, func() (*exec.Cmd, error) {
				return nil, tasker.DeleteTask(m.root, selectedID, recursive)
			})
		}
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
	m.syncCurrentViewport(false)
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
	focus := metaStyle.Render(fmt.Sprintf(
		"focus %s=current %s=tasks %s=workers",
		keyLabel(m.keybindings.Global, "focus_current"),
		keyLabel(m.keybindings.Global, "focus_tasks"),
		keyLabel(m.keybindings.Global, "focus_workers"),
	))
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
		keyStyle.Render(fmt.Sprintf("%s/%s/%s focus panels", keyLabel(m.keybindings.Global, "focus_current"), keyLabel(m.keybindings.Global, "focus_tasks"), keyLabel(m.keybindings.Global, "focus_workers"))),
		keyStyle.Render(fmt.Sprintf("%s filter", keyLabel(m.keybindings.Global, "filter"))),
		keyStyle.Render(fmt.Sprintf("%s status filter", keyLabel(m.keybindings.Global, "cycle_status_filter"))),
		keyStyle.Render(fmt.Sprintf("%s type filter", keyLabel(m.keybindings.Global, "cycle_type_filter"))),
		keyStyle.Render(fmt.Sprintf("%s open task/output  %s/%s delete/stop", keyLabel(m.keybindings.Tasks, "open_current"), keyLabel(m.keybindings.Tasks, "delete_task"), keyLabel(m.keybindings.Workers, "stop_task"))),
		keyStyle.Render(fmt.Sprintf("%s/%s/%s/%s/%s switch current view", keyLabel(m.keybindings.Current, "show_task"), keyLabel(m.keybindings.Current, "show_result"), keyLabel(m.keybindings.Current, "show_status"), keyLabel(m.keybindings.Current, "show_diff"), keyLabel(m.keybindings.Current, "show_agent"))),
		keyStyle.Render(fmt.Sprintf("%s edit  %s do  %s resume  %s fork", keyLabel(m.keybindings.Current, "edit_doc"), keyLabel(m.keybindings.Current, "run_do"), keyLabel(m.keybindings.Current, "resume"), keyLabel(m.keybindings.Current, "fork_session"))),
		keyStyle.Render(fmt.Sprintf("%s refresh", keyLabel(m.keybindings.Global, "refresh"))),
		keyStyle.Render(fmt.Sprintf("%s help", keyLabel(m.keybindings.Global, "toggle_help"))),
		keyStyle.Render(fmt.Sprintf("%s quit", keyLabel(m.keybindings.Global, "quit"))),
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

func (m *model) syncCurrentViewport(forceTop bool) {
	taskID := ""
	if task := m.selectedTask(); task != nil {
		taskID = task.Meta.ID
	}
	if !forceTop && (taskID != m.currentViewportTaskID || m.currentViewMode != m.currentViewportView) {
		forceTop = true
	}
	m.currentViewport.SetContent(m.currentContent())
	if forceTop {
		m.currentViewport.GotoTop()
	}
	m.currentViewportTaskID = taskID
	m.currentViewportView = m.currentViewMode
}

func (m model) currentContent() string {
	task := m.selectedTask()
	if task == nil {
		return "No task selected."
	}

	mode := m.currentViewMode
	if mode == viewAuto {
		if task.Status.Status == "RUNNING" {
			return m.decorateCurrentContent(*task, "agent output", m.agentOutputContent(*task))
		}
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
	case viewDiff:
		return m.decorateCurrentContent(*task, "git diff", m.repoDiffContent(*task))
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

func (m model) repoDiffContent(task tasker.Task) string {
	repo, err := tasker.OpenGitRepo(m.root)
	if err != nil {
		return "Git diff unavailable: " + err.Error()
	}

	diff, err := repo.TaskDiff(&task)
	if err != nil {
		return "Error loading git diff: " + err.Error()
	}
	if len(diff) == 0 {
		return "No task-scoped changes yet."
	}
	return renderSideBySideDiffContent(diff, maxInt(m.currentViewport.Width, 80))
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
		source := fmt.Sprintf("Source: %s session %s", session.Agent, session.ID)
		if strings.TrimSpace(session.ID) == "" {
			source = "Source: task live output"
		}
		lines = append(lines, source)
		if session.RecordedAt != "" {
			lines = append(lines, "Recorded: "+session.RecordedAt)
		}
		lines = append(lines, "", output)
		return strings.Join(lines, "\n")
	}

	if task.Status.Status == "RUNNING" {
		return strings.Join([]string{
			"Waiting for Codex session output...",
			"The task is still running, but no readable agent transcript is available yet.",
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

func (m *model) resumeSelectedTask(fork bool) (tea.Model, tea.Cmd) {
	task := m.selectedTask()
	if task == nil {
		return m.withError("select a task first"), nil
	}

	cmd, err := tasker.ResumeTaskCommand(m.root, task, fork)
	if err != nil {
		return m.withError(err.Error()), nil
	}

	status := "Resumed task " + task.Meta.ID
	if fork {
		status = "Forked task " + task.Meta.ID
	}
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return externalDoneMsg{
			Status:     status,
			SelectedID: task.Meta.ID,
			Err:        err,
			Refresh:    true,
		}
	})
}

func (m model) renderHelp() string {
	lines := []string{
		"Global",
		fmt.Sprintf("%s/%s/%s focus current, tasks, and workers panels", keyLabel(m.keybindings.Global, "focus_current"), keyLabel(m.keybindings.Global, "focus_tasks"), keyLabel(m.keybindings.Global, "focus_workers")),
		fmt.Sprintf("%s focuses the task filter", keyLabel(m.keybindings.Global, "filter")),
		fmt.Sprintf("%s cycles status filters, %s cycles type filters", keyLabel(m.keybindings.Global, "cycle_status_filter"), keyLabel(m.keybindings.Global, "cycle_type_filter")),
		fmt.Sprintf("%s refreshes data, %s toggles help, %s quits", keyLabel(m.keybindings.Global, "refresh"), keyLabel(m.keybindings.Global, "toggle_help"), keyLabel(m.keybindings.Global, "quit")),
		"",
		"Tasks",
		fmt.Sprintf("%s/%s move selection", keyLabel(m.keybindings.Tasks, "move_up"), keyLabel(m.keybindings.Tasks, "move_down")),
		fmt.Sprintf("%s toggles subtasks and opens the current-view panel", keyLabel(m.keybindings.Tasks, "open_current")),
		fmt.Sprintf("%s new task, %s add child, %s edit meta, %s checkout", keyLabel(m.keybindings.Tasks, "new_task"), keyLabel(m.keybindings.Tasks, "add_child"), keyLabel(m.keybindings.Tasks, "edit_meta"), keyLabel(m.keybindings.Tasks, "checkout")),
		fmt.Sprintf("%s import tasks, %s create import template, %s delete", keyLabel(m.keybindings.Tasks, "import_tasks"), keyLabel(m.keybindings.Tasks, "create_import_template"), keyLabel(m.keybindings.Tasks, "delete_task")),
		fmt.Sprintf("%s open task/project documents in the configured editor", keyLabel(m.keybindings.Tasks, "open_doc")),
		fmt.Sprintf("%s opens the selected task's agent output", keyLabel(m.keybindings.Tasks, "open_output")),
		fmt.Sprintf("%s run `tasker do`, %s run `tasker resume`, %s fork a stored session", keyLabel(m.keybindings.Tasks, "run_do"), keyLabel(m.keybindings.Tasks, "resume"), keyLabel(m.keybindings.Tasks, "fork_session")),
		"",
		"Current View",
		fmt.Sprintf("%s shows task.md, %s shows result.md, %s shows status, %s shows git diff, %s/%s show agent output", keyLabel(m.keybindings.Current, "show_task"), keyLabel(m.keybindings.Current, "show_result"), keyLabel(m.keybindings.Current, "show_status"), keyLabel(m.keybindings.Current, "show_diff"), keyLabel(m.keybindings.Current, "show_agent"), keyLabel(m.keybindings.Current, "open_output")),
		fmt.Sprintf("%s opens the selected task or result in your editor", keyLabel(m.keybindings.Current, "edit_doc")),
		fmt.Sprintf("%s run `tasker do`, %s run `tasker resume`, %s fork a stored session", keyLabel(m.keybindings.Current, "run_do"), keyLabel(m.keybindings.Current, "resume"), keyLabel(m.keybindings.Current, "fork_session")),
		"",
		"Workers",
		fmt.Sprintf("%s/%s move between running tasks", keyLabel(m.keybindings.Workers, "move_up"), keyLabel(m.keybindings.Workers, "move_down")),
		fmt.Sprintf("%s opens the selected task's agent output", keyLabel(m.keybindings.Workers, "open_output")),
		fmt.Sprintf("%s confirms stopping the selected running task", keyLabel(m.keybindings.Workers, "stop_task")),
		"",
		"Forms",
		fmt.Sprintf("%s moves between fields", keyLabel(m.keybindings.Form, "next_field")),
		fmt.Sprintf("%s/%s cycle select fields", keyLabel(m.keybindings.Form, "prev_option"), keyLabel(m.keybindings.Form, "next_option")),
		fmt.Sprintf("%s submits, %s cancels", keyLabel(m.keybindings.Form, "submit"), keyLabel(m.keybindings.Form, "cancel")),
		"",
		"Confirmations",
		fmt.Sprintf("%s toggle confirm", keyLabel(m.keybindings.Confirm, "toggle_choice")),
		fmt.Sprintf("%s toggles recursive delete when deleting tasks", keyLabel(m.keybindings.Confirm, "toggle_recursive")),
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
	lines = append(lines, fmt.Sprintf("%s submit  %s cancel", keyLabel(m.keybindings.Form, "submit"), keyLabel(m.keybindings.Form, "cancel")))
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
	lines = append(lines, "", fmt.Sprintf("%s run  %s cancel", keyLabel(m.keybindings.Session, "select"), keyLabel(m.keybindings.Session, "cancel")))
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
	}
	if m.confirm.Kind == modalDelete {
		lines = append(lines,
			fmt.Sprintf("Recursive delete: %t  (press %s to toggle)", m.confirm.Recursive, keyLabel(m.keybindings.Confirm, "toggle_recursive")),
			"",
		)
	}
	lines = append(lines, choiceNo+"   "+choiceYes)
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
	case "CANCELLED", "BLOCKED":
		color = lipgloss.Color("196")
	case "HANDOFF", "REVIEW", "AWAITING_ACTION":
		color = lipgloss.Color("170")
	}

	return lipgloss.NewStyle().Bold(true).Foreground(color).Render(badge)
}

func renderDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117")).Render(line)
	case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "), strings.HasPrefix(line, "@@"):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("228")).Render(line)
	case strings.HasPrefix(line, "+"):
		return lipgloss.NewStyle().Background(lipgloss.Color("22")).Foreground(lipgloss.Color("230")).Render(line)
	case strings.HasPrefix(line, "-"):
		return lipgloss.NewStyle().Background(lipgloss.Color("52")).Foreground(lipgloss.Color("230")).Render(line)
	case strings.HasPrefix(line, "Untracked files:"):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render(line)
	default:
		return line
	}
}

type sideBySideRow struct {
	left       string
	right      string
	leftKind   string
	rightKind  string
	showNumber bool
	leftLine   int
	rightLine  int
}

func renderSideBySideDiffContent(files []tasker.TaskFileDiff, width int) string {
	if len(files) == 0 {
		return "No task-scoped changes yet."
	}

	totalWidth := maxInt(width, 80)
	gutter := " | "
	columnWidth := maxInt((totalWidth-len(gutter))/2, 20)
	lines := make([]string, 0, len(files)*8)
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("228"))

	for i, file := range files {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, headerStyle.Render("File: "+file.Path))
		lines = append(lines, renderSideBySideHeader(columnWidth, gutter))
		for _, row := range buildSideBySideRows(file.Before, file.After) {
			lines = append(lines, renderSideBySideRow(row, columnWidth, gutter))
		}
	}

	return strings.Join(lines, "\n")
}

func renderSideBySideHeader(columnWidth int, gutter string) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	left := fitSideBySideText("OLD", columnWidth)
	right := fitSideBySideText("NEW", columnWidth)
	return header.Render(left + gutter + right)
}

func buildSideBySideRows(before, after string) []sideBySideRow {
	beforeLines := splitDiffLines(before)
	afterLines := splitDiffLines(after)
	ops := diffLineOps(beforeLines, afterLines)
	ranges := diffDisplayRanges(ops, 2)
	if len(ranges) == 0 {
		return []sideBySideRow{{
			left:      "(no text changes)",
			right:     "(no text changes)",
			leftKind:  "meta",
			rightKind: "meta",
		}}
	}

	rows := make([]sideBySideRow, 0, len(ops))
	for idx, bounds := range ranges {
		if idx > 0 {
			rows = append(rows, sideBySideRow{
				left:      "...",
				right:     "...",
				leftKind:  "meta",
				rightKind: "meta",
			})
		}

		leftLine, rightLine := 1, 1
		for i := 0; i < bounds.start; i++ {
			switch ops[i].kind {
			case "equal":
				leftLine++
				rightLine++
			case "delete":
				leftLine++
			case "insert":
				rightLine++
			}
		}

		for i := bounds.start; i < bounds.end; {
			if ops[i].kind == "equal" {
				rows = append(rows, sideBySideRow{
					left:       ops[i].left,
					right:      ops[i].right,
					leftKind:   "equal",
					rightKind:  "equal",
					showNumber: true,
					leftLine:   leftLine,
					rightLine:  rightLine,
				})
				leftLine++
				rightLine++
				i++
				continue
			}

			blockStart := i
			for i < bounds.end && ops[i].kind != "equal" {
				i++
			}
			block := ops[blockStart:i]
			deletes := make([]string, 0, len(block))
			inserts := make([]string, 0, len(block))
			for _, op := range block {
				switch op.kind {
				case "delete":
					deletes = append(deletes, op.left)
				case "insert":
					inserts = append(inserts, op.right)
				}
			}
			blockRows := maxInt(len(deletes), len(inserts))
			for j := 0; j < blockRows; j++ {
				row := sideBySideRow{showNumber: true}
				if j < len(deletes) {
					row.left = deletes[j]
					row.leftKind = "delete"
					row.leftLine = leftLine
					leftLine++
				}
				if j < len(inserts) {
					row.right = inserts[j]
					row.rightKind = "insert"
					row.rightLine = rightLine
					rightLine++
				}
				rows = append(rows, row)
			}
		}
	}

	return rows
}

type lineOp struct {
	kind  string
	left  string
	right string
}

type displayRange struct {
	start int
	end   int
}

func diffLineOps(before, after []string) []lineOp {
	lcs := buildLCSMatrix(before, after)
	ops := make([]lineOp, 0, len(before)+len(after))
	i, j := 0, 0
	for i < len(before) && j < len(after) {
		switch {
		case before[i] == after[j]:
			ops = append(ops, lineOp{kind: "equal", left: before[i], right: after[j]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, lineOp{kind: "delete", left: before[i]})
			i++
		default:
			ops = append(ops, lineOp{kind: "insert", right: after[j]})
			j++
		}
	}
	for ; i < len(before); i++ {
		ops = append(ops, lineOp{kind: "delete", left: before[i]})
	}
	for ; j < len(after); j++ {
		ops = append(ops, lineOp{kind: "insert", right: after[j]})
	}
	return ops
}

func buildLCSMatrix(before, after []string) [][]int {
	matrix := make([][]int, len(before)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(after)+1)
	}
	for i := len(before) - 1; i >= 0; i-- {
		for j := len(after) - 1; j >= 0; j-- {
			if before[i] == after[j] {
				matrix[i][j] = matrix[i+1][j+1] + 1
			} else {
				matrix[i][j] = maxInt(matrix[i+1][j], matrix[i][j+1])
			}
		}
	}
	return matrix
}

func diffDisplayRanges(ops []lineOp, context int) []displayRange {
	ranges := make([]displayRange, 0, 4)
	for i, op := range ops {
		if op.kind == "equal" {
			continue
		}
		start := maxInt(i-context, 0)
		end := minInt(i+context+1, len(ops))
		if len(ranges) == 0 || start > ranges[len(ranges)-1].end {
			ranges = append(ranges, displayRange{start: start, end: end})
			continue
		}
		if end > ranges[len(ranges)-1].end {
			ranges[len(ranges)-1].end = end
		}
	}
	return ranges
}

func renderSideBySideRow(row sideBySideRow, columnWidth int, gutter string) string {
	leftText := row.left
	rightText := row.right
	if row.showNumber {
		if row.leftKind != "" {
			leftText = strconv.Itoa(row.leftLine) + " " + leftText
		}
		if row.rightKind != "" {
			rightText = strconv.Itoa(row.rightLine) + " " + rightText
		}
	}

	left := styleSideBySideCell(fitSideBySideText(leftText, columnWidth), row.leftKind)
	right := styleSideBySideCell(fitSideBySideText(rightText, columnWidth), row.rightKind)
	return left + gutter + right
}

func fitSideBySideText(text string, width int) string {
	text = strings.ReplaceAll(text, "\t", "    ")
	if runewidth.StringWidth(text) > width {
		return runewidth.Truncate(text, width-1, "…")
	}
	padding := width - runewidth.StringWidth(text)
	if padding <= 0 {
		return text
	}
	return text + strings.Repeat(" ", padding)
}

func styleSideBySideCell(text, kind string) string {
	style := lipgloss.NewStyle()
	switch kind {
	case "delete":
		style = style.Background(lipgloss.Color("52")).Foreground(lipgloss.Color("230"))
	case "insert":
		style = style.Background(lipgloss.Color("22")).Foreground(lipgloss.Color("230"))
	case "equal":
		style = style.Foreground(lipgloss.Color("245"))
	case "meta":
		style = style.Foreground(lipgloss.Color("241")).Italic(true)
	}
	return style.Render(text)
}

func splitDiffLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
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
