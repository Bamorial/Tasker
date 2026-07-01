# Tasker

Tasker is a CLI-first task workspace for human and AI collaboration.

It turns work into a durable file-based protocol instead of a chat-only workflow. Every task gets its own folder, goal, instructions, handoff notes, result report, and machine-readable status so another person or another agent can continue the work without guessing what happened before.

Tasker is useful when you want:

- explicit task ownership and status
- clean handoffs between humans and agents
- task context that survives editor restarts, branch changes, and lost chat history
- a simple workflow that lives inside a normal Git repository

This project is also maintained with Tasker itself.

## What Tasker Creates

Running `tasker init` creates a `.tasker/` workspace plus an `AGENTS.md` file.

Core workspace files:

- `.tasker/START.md`: how an agent should begin work
- `.tasker/instructions.md`: repository-wide rules
- `.tasker/current/WORKSPACE.md`: current active task summary
- `.tasker/current/FILES.md`: relevant files for the active task
- `.tasker/current/CONTEXT.json`: machine-readable current task context
- `.tasker/tasks/`: every task and subtask folder

Each task folder contains:

- `task.md`: title, metadata summary, and goal
- `instructions.md`: task-specific rules
- `declaration.md`: work-in-progress handoff
- `result.md`: final summary
- `meta.json`: task metadata
- `status.json`: machine-readable status
- `context.json`: task-local structured context
- `children/`: child tasks
- `sessions/`: session artifacts

When Tasker detects an active agent session while creating a task, it stores a machine-readable session index at `sessions/index.json` and mirrors that session metadata into `status.json`.

Tasker recognizes these task status values in `status.json`: `NEW`, `PLANNED`, `RUNNING`, `IN_PROGRESS`, `HANDOFF`, `AWAITING_ACTION`, `REVIEW`, `BLOCKED`, `CANCELLED`, and `DONE`.

Agents should not create task folders manually under `.tasker/tasks/`. When a new task is needed, use `tasker new`, `tasker add`, or `tasker import` so Tasker can keep metadata, workspace state, and session tracking consistent.

## Core Ideas

### 1. Tasks are files, not chat state

Tasker assumes work should be resumable from disk. The task folder is the source of truth.

### 2. Parent-child task chains matter

Child tasks live under a parent task and inherit context from that chain. When a task is checked out, Tasker writes the parent chain into the current workspace files so the next worker knows what to read.

### 3. Checkout means task context, not only Git

`tasker checkout <id>` updates `.tasker/current/*` for the selected task. If Git integration is enabled, it can also create or switch branches for that task.

### 4. Git support is optional

Tasker works without Git branch automation. If enabled, it can use one branch per task and write the current branch into workspace context.

## Current Commands

Tasker currently implements:

- `tasker init`
- `tasker new [title]`
- `tasker add [title]`
- `tasker import [path]`
- `tasker import template`
- `tasker checkout <id>`
- `tasker do <id>`
- `tasker open <id>`
- `tasker resume <id>`
- `tasker instruction <id>`
- `tasker instructions`
- `tasker meta <id>`
- `tasker delete <id>`
- `tasker tree`
- `tasker status`
- `tasker status <id>`
- `tasker version`

Running `tasker` with no arguments opens the native Tasker terminal UI. Use `tasker help` to see the command list.

## Installation And Build

Requirements:

- Go 1.22+

Build locally:

```bash
go mod tidy
go build ./...
```

Helper scripts:

```bash
./scripts/build.sh
./scripts/install.sh
```

Versioned local build:

```bash
VERSION=v0.1.0 COMMIT=local DATE=2026-06-29T00:00:00Z ./scripts/build.sh
./bin/tasker version
```

Global install behavior:

- `./scripts/install.sh` builds `bin/tasker` and symlinks it to `/opt/homebrew/bin/tasker`
- override the install target with `TASKER_INSTALL_DIR=/some/path ./scripts/install.sh`

## Quick Start

Initialize Tasker in an existing repository:

```bash
tasker init
```

Create a top-level task:

```bash
tasker new "Add Git worktree support"
```

Create a task and immediately make it the current workspace:

```bash
tasker new -c "Write release checklist"
```

Create a task, check it out, and switch to its task branch:

```bash
tasker new -bc "Fix broken CLI status colors"
```

Create a child task under the current task:

```bash
tasker add "Document fallback behavior"
```

Create an editable import file:

```bash
tasker import template
```

Import the most recent working copy from `.tasker/imports/`:

```bash
tasker import
```

Import a specific JSON file:

```bash
cp .tasker/templates/import-tasks.json /tmp/my-tasks.json
tasker import /tmp/my-tasks.json
```

See the task tree:

```bash
tasker tree
```

See status for everything:

```bash
tasker status
```

Open the terminal UI:

```bash
tasker
```

## Configuration

Tasker reads `.tasker/config.yaml`.

Default config:

```yaml
editor: ""

git:
  enabled: false
  branch_per_task: true
  checkout_branch: false
  commit_per_subtask: true
  branch_prefix: "task"

tui:
  keybindings:
    global:
      quit: ["ctrl+c", "q"]
      focus_current: ["0"]
      focus_tasks: ["1"]
      focus_workers: ["2"]
      toggle_help: ["?"]
      refresh: ["R"]
      filter: ["/"]
      cycle_status_filter: ["S"]
      cycle_type_filter: ["T"]
    tasks:
      move_up: ["up", "k"]
      move_down: ["down", "j"]
      page_up: ["pgup"]
      page_down: ["pgdown"]
      new_task: ["n"]
      add_child: ["a"]
      edit_meta: ["m"]
      checkout: ["c"]
      import_tasks: ["u"]
      create_import_template: ["I"]
      delete_task: ["d"]
      open_doc: ["e"]
      run_do: ["x"]
      resume: ["*"]
      fork_session: ["f"]
      open_output: ["o"]
      open_current: ["enter"]
    current:
      show_task: ["t"]
      show_result: ["r"]
      show_status: ["s"]
      show_agent: ["w"]
      open_output: ["o"]
      edit_doc: ["e"]
      run_do: ["x"]
      resume: ["*"]
      fork_session: ["F"]
    workers:
      move_up: ["up", "k"]
      move_down: ["down", "j"]
      page_up: ["pgup"]
      page_down: ["pgdown"]
      open_output: ["enter"]
      stop_task: ["d"]
    viewport:
      line_up: ["up", "k", "ctrl+p"]
      line_down: ["down", "j", "ctrl+n"]
      page_up: ["pgup", "b"]
      page_down: ["pgdown", "space"]
      top: ["g", "home"]
      bottom: ["G", "end"]
    filter:
      cancel: ["esc"]
      apply: ["enter"]
    form:
      cancel: ["esc"]
      next_field: ["tab", "down"]
      prev_field: ["shift+tab", "up"]
      prev_option: ["left"]
      next_option: ["right"]
      submit: ["ctrl+s"]
      submit_or_next: ["enter"]
    session:
      cancel: ["esc"]
      move_up: ["up", "k"]
      move_down: ["down", "j"]
      select: ["enter"]
    confirm:
      cancel: ["esc"]
      toggle_choice: ["left", "h", "right", "l"]
      toggle_recursive: ["r"]
      accept: ["enter"]
```

Behavior notes:

- `editor` is checked before `$EDITOR`
- if neither is set, editor-opening commands print the target path instead
- `git.checkout_branch: true` allows `tasker checkout <id>` to auto-create or reuse the generated branch
- `git.branch_prefix` defaults to `task`
- `tui.keybindings` overrides the built-in Tasker TUI shortcuts by action name
- any omitted keybinding actions fall back to the defaults above

Generated branch format:

```text
<branch_prefix>/<task-id>-<task-slug>
```

Example:

```text
task/013-automatical-checkout
```

## Command Guide

### `tasker init`

Initializes Tasker in the current repository.

Behavior:

- creates the `.tasker/` workspace directories if missing
- creates starter templates like `AGENTS.md`, `.tasker/START.md`, and `.tasker/config.yaml`
- creates an editable task import template at `.tasker/templates/import-tasks.json`
- creates `.tasker/imports/` for editable import working copies
- creates editable task templates under `.tasker/templates/tasks/`
- does not overwrite existing files that are already present

Example:

```bash
tasker init
```

### `tasker new [title]`

Creates a top-level task.

Options:

- `--type <type>`: task type, one of `bug`, `decision`, `documentation`, `feature`, `research`, `review`, `test`
- `--open <target>`: open `task`, `instructions`, `declaration`, `result`, or `meta`
- `--no-open`: create the task without opening an editor
- `-c`, `--checkout`: create the task and set it as the current workspace without switching Git branches
- `-b`, `--branch-checkout`: create the task, set it as current, and create or switch to its task branch

Behavior:

- if no title is given, Tasker uses `Untitled task`
- if `--type` is omitted, Tasker defaults the task type to `feature` and uses `.tasker/templates/tasks/feature.md`
- if `--type` is provided, Tasker uses `.tasker/templates/tasks/<type>.md`
- IDs are global across the whole workspace, not only top-level tasks
- `-c` updates `.tasker/current/WORKSPACE.md`, `.tasker/current/FILES.md`, and `.tasker/current/CONTEXT.json`
- `-b` forces task-branch checkout even if `git.checkout_branch` is disabled in config
- `tasker new -bc "Title"` works because Cobra accepts combined short flags

Examples:

```bash
tasker new "Design status view"
tasker new --type research "Investigate old document format"
tasker new --open declaration "Refactor startup flow"
tasker new --no-open "Write docs"
tasker new -c "Prepare migration checklist"
tasker new -bc "Implement automatic checkout"
```

### `tasker add [title]`

Creates a child task under an existing task.

Options:

- `--parent <id>`: explicit parent task ID
- `--type <type>`: task type
- `--open <target>`: open `task`, `instructions`, `declaration`, `result`, or `meta`
- `--no-open`: create the task without opening an editor

Behavior:

- if `--parent` is omitted, Tasker tries to infer the parent from the current directory or `.tasker/current/CONTEXT.json`
- if `--type` is omitted, Tasker defaults the task type to `feature` and uses `.tasker/templates/tasks/feature.md`
- if `--type` is provided, Tasker uses `.tasker/templates/tasks/<type>.md`
- child tasks are written under `<parent>/children/`
- IDs are still global

Examples:

```bash
tasker add --parent 006 "Add tests for branch naming"
tasker add "Split status output formatting"
tasker add --type documentation --open instructions "Describe checkout behavior"
```

### `tasker import [path]`

Imports tasks from a JSON document on disk.

Options:

- `--parent <id>`: attach all imported root tasks under an existing parent task
- `--open <target>`: open `task`, `instructions`, `declaration`, `result`, or `meta`
- `--no-open`: import the task without opening an editor
- `-c`, `--checkout`: import the tasks and set the first imported root task as the current workspace without switching Git branches
- `-b`, `--branch-checkout`: import the tasks, set the first imported root task as current, and create or switch to its task branch

Path behavior:

- if a path is passed, Tasker imports that file
- if no path is passed, Tasker imports the most recently modified `.json` file from `.tasker/imports/`
- if `.tasker/imports/` has no import files yet, Tasker tells you to run `tasker import template` first

Import document format:

- the document is a JSON object with a top-level `tasks` array
- each task object can declare `title`, `type`, `body`, `instructions`, `declaration`, `result`, `context`, and `subtasks`
- `subtasks` is recursive, so one file can define a whole task tree
- `type` is optional and defaults to `feature`
- `body` becomes `task.md` when it is present
- the optional document fields overwrite their matching Tasker files when provided

Behavior:

- Tasker creates every declared task and subtask in order
- nested `subtasks` become child task folders
- if a text field is empty, Tasker keeps the default generated file content
- if `context` is provided, Tasker writes it to `context.json`
- `.tasker/templates/import-tasks.json` provides the import shape to copy and edit
- `-c`, `-b`, and `--open` operate on the first imported root task

Example:

```bash
tasker import
```

### `tasker import template`

Creates a copy of `.tasker/templates/import-tasks.json` inside `.tasker/imports/` and opens it in the configured editor.

Behavior:

- creates a timestamped file like `.tasker/imports/import-20260630-153045.json`
- copies the template shape exactly
- opens the new file in the configured editor
- prints the file path if no editor is configured or opening fails

Example:

```bash
tasker import template
```

### `tasker checkout <id>`

Sets the current task workspace and can also switch Git branches.

Options:

- `--branch <name>`: create or reuse this branch for the task
- `--existing-branch <name>`: link the task to an already existing branch
- `--no-branch`: update current workspace files without switching Git branches
- `--print-path`: print the task directory path for shell wrappers

Behavior:

- always writes current task context into `.tasker/current/*`
- includes parent task chain information for child tasks
- if Git is enabled and `git.checkout_branch: true`, default checkout creates or switches to the generated task branch
- `--branch` overrides the generated task branch name
- `--existing-branch` fails if the branch does not exist
- `--no-branch` cannot be combined with branch-selection options

Examples:

```bash
tasker checkout 013
tasker checkout 013 --no-branch
tasker checkout 013 --branch feature/custom-name
tasker checkout 013 --existing-branch feature/manual-link
tasker checkout 013 --print-path
```

### `tasker`

Opens the native Tasker terminal UI.

Behavior:

- loads tasks, status counts, and the current workspace directly from `internal/tasker`
- shows a single lazygit-style screen with three numbered panels
- keeps the task tree and worker output in the left column and the selected task view on the right
- supports keyboard-driven `new`, `add`, `meta`, `checkout`, `import`, `import template`, `delete`, `stop`, `do`, `resume`, and `fork` flows
- uses external subprocesses for editor launches, detached `tasker do` runs, and stored session resume/fork commands
- refreshes the task tree, current view, and worker panes after mutating actions
- watches `.tasker/` for external file changes so task status, results, and live output stay in sync without manual refresh
- can read a running task's Codex transcript from `sessions/execution.json` plus the persisted `~/.codex/sessions` data even before the task-local session index is written
- defaults `Open editor` to `open` in task/import forms when an editor is configured via `.tasker/config.yaml` or `$EDITOR`

Key workflows:

- all defaults come from `.tasker/config.yaml` under `tui.keybindings`
- `global` controls panel focus, help, refresh, filtering, and filter cycling
- `tasks` controls task-tree navigation plus task actions like new, add, checkout, delete, do, resume, fork, and open output
- `current` controls the right-hand panel view switching and task actions
- `workers` controls the running-task list navigation, output opening, and stop confirmation entrypoint
- `viewport`, `filter`, `form`, `session`, and `confirm` control the shared modal and scrolling shortcuts

Example:

```bash
tasker
```

### `tasker open <id>`

Opens a task's `task.md` in the configured editor.

Behavior:

- uses `.tasker/config.yaml` `editor` first
- falls back to `$EDITOR`
- prints the file path if no editor is configured or opening fails

Example:

```bash
tasker open 013
```

### `tasker resume <id>`

Resumes or forks a stored agent session for a task.

Options:

- `-f`, `--fork`: fork the stored session instead of resuming it

Behavior:

- reads stored sessions from the task's `status.json`
- uses the stored resume command by default
- uses the stored fork command when `-f` is passed
- launches the selected session command attached to the current terminal
- prompts you to choose a session when multiple stored sessions match the requested action

Examples:

```bash
tasker resume 018
tasker resume -f 018
```

### `tasker do <id>`

Runs a task in a new headless Codex session and stores the created session on the task.

Behavior:

- refreshes `.tasker/current/*` for the target task
- marks the task `RUNNING` before the headless session starts
- runs `codex exec` from the repository root in headless mode
- suppresses the noisy `Reading additional input from stdin...` notice from the underlying exec process
- captures the new session ID from the machine-readable exec stream when available
- falls back to the persisted `~/.codex/sessions` metadata for matching `codex_exec` runs when the stream does not expose `session_meta`
- writes enough execution metadata for the TUI to find the matching persisted transcript while the run is still in progress
- stores the session in both `status.json` and `sessions/index.json`
- preserves any final task status written by the agent, and otherwise promotes the successful run to `DONE`
- leaves the task resumable later with `tasker resume <id>`

Example:

```bash
tasker do 022
```

### `tasker instruction <id>`

Opens a task's `instructions.md`.

Example:

```bash
tasker instruction 013
```

### `tasker instructions`

Opens the project-wide `.tasker/instructions.md`.

## Task Templates

`tasker init` creates an editable import template at `.tasker/templates/import-tasks.json`, an `.tasker/imports/` workspace for working copies, and task templates in `.tasker/templates/tasks/`:

- `default.md`
- `bug.md`
- `decision.md`
- `documentation.md`
- `feature.md`
- `research.md`
- `review.md`
- `test.md`

Tasker replaces these placeholders when it creates `task.md`:

- `{{TITLE}}`
- `{{ID}}`
- `{{TYPE}}`
- `{{CREATED_AT}}`

If a template file is missing, Tasker falls back to its built-in default for that template.

Example:

```bash
tasker instructions
```

### `tasker meta <id>`

Opens or updates a task's metadata.

Options:

- `--title <title>`: update the task title
- `--type <type>`: update the task type
- `--open`: open `meta.json` after applying updates

Behavior:

- if you only run `tasker meta <id>`, it opens `meta.json`
- if you update metadata without `--open`, it prints a confirmation and exits
- renaming a task updates the metadata and the task folder slug

Examples:

```bash
tasker meta 013
tasker meta 013 --title "automatic checkout"
tasker meta 013 --type documentation --open
```

### `tasker delete <id>`

Deletes a task.

Options:

- `--recursive`: delete the task and all child tasks

Behavior:

- deleting a leaf task works directly
- deleting a task with children fails unless `--recursive` is set

Examples:

```bash
tasker delete 014
tasker delete 006 --recursive
```

### `tasker tree`

Prints the task hierarchy.

Behavior:

- top-level tasks are shown first
- child tasks are indented underneath their parent
- each line includes the task ID, title, and type

Example:

```bash
tasker tree
```

### `tasker status [id]`

Shows status output for the whole workspace or one task.

Behavior with no ID:

- prints the full task tree
- includes task status, title, type, agent, and child count metadata

Behavior with an ID:

- shows one task in detail
- includes status, type, agent, created time, started time, and path
- includes stored agent session IDs plus resume or fork commands when known
- shows subtasks
- shows notes from `task.md`, `instructions.md`, `declaration.md`, and `result.md`

Formatting behavior:

- uses ANSI color when output is a terminal
- disables color when `NO_COLOR` is set
- disables color when `TERM=dumb`
- falls back to plain text when redirected

Examples:

```bash
tasker status
tasker status 013
NO_COLOR=1 tasker status
```

### `tasker version`

Shows build version information.

Example:

```bash
tasker version
```

## Typical Workflows

### Solo development with explicit task history

```bash
tasker init
tasker new -c "Refactor auth middleware"
tasker status
tasker open 001
```

Use this when you want lightweight task tracking inside a normal repository without adding a separate project management system.

### Human-to-agent handoff

```bash
tasker new -c "Write migration plan"
tasker instruction 001
tasker status 001
```

Use this when a human defines the task and an agent continues the implementation from the on-disk task files.

### Parent task with focused subtasks

```bash
tasker new -c "Ship status redesign"
tasker add "Update rendering logic"
tasker add "Document color behavior"
tasker tree
```

Use this when one larger feature should be split into smaller units that still preserve shared context.

### One branch per task

```bash
tasker new -bc "Add release automation"
tasker checkout 002
tasker checkout 003 --existing-branch feature/manual-branch
```

Use this when each task should have isolated Git work tied to its task ID and slug.

## Practical Notes

- Task IDs are zero-padded and global across the workspace.
- Child tasks live under the parent task folder, not only in a flat global list.
- `tasker checkout` is the command that refreshes `.tasker/current/*`.
- `tasker new -c` and `tasker new -b` are shortcuts for creating a task and immediately making it active.
- Tasker does not require Git branch automation to be useful.
- Opening files depends on either `.tasker/config.yaml` `editor` or `$EDITOR`.
- If `CODEX_THREAD_ID` is present, new tasks store it and show `codex resume <id>` and `codex fork <id>` in `tasker status <id>`.
- `tasker do <id>` starts a fresh headless `codex exec` run, and Tasker can recover its transcript/session from persisted `codex_exec` metadata when the live exec stream does not expose that data immediately.
- `tasker resume <id>` uses the stored session commands and prompts when more than one session can be resumed or forked.
- For other agents, you can inject session metadata with `TASKER_SESSION_ID`, `TASKER_SESSION_AGENT`, `TASKER_SESSION_RESUME_COMMAND`, and `TASKER_SESSION_FORK_COMMAND`.

## Packaging

- tagged releases are configured through `.goreleaser.yaml`
- GitHub Actions release automation lives in `.github/workflows/release.yml`
- Debian package generation is configured through GoReleaser NFPM
- maintainer release workflow is documented in `docs/releasing.md`
