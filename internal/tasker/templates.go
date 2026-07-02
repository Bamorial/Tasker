package tasker

import "fmt"

func agentsTemplate() string {
	return `This repository uses Tasker.

Before working:

1. Read .tasker/START.md
2. Read .tasker/current/WORKSPACE.md if it exists
3. Read .tasker/instructions.md
4. Read the current task folder, and if the task is a child task, read its parent task chain as well

All important communication must happen through:

- declaration.md
- result.md
- status.json

Never create task folders or files manually under .tasker/tasks. Use tasker new, tasker add, or tasker import.

Never leave undocumented changes.
`
}

func startTemplate() string {
	return `# Tasker Start

Tasker stores work as files so any human or AI agent can continue without prior chat history.

Read order:

1. .tasker/instructions.md for project-wide rules
2. .tasker/current/WORKSPACE.md for active workspace context
3. The current task folder for task-specific details
4. If the current task is a child task, read the parent task chain before starting work

Task model:

- Each task lives in .tasker/tasks/
- Each task folder is the source of truth for goals, status, decisions, and results
- Child tasks live under the parent task's children/ directory

Agent communication:

- declaration.md is the working handoff
- result.md is the final report
- status.json is the machine-readable state

Task creation rule:

- Do not manually create task folders or files under .tasker/tasks/
- Use tasker new, tasker add, or tasker import when a new task is needed
`
}

func instructionsTemplate() string {
	return `# Project Instructions

Document repository-wide rules here.

Examples:

- Architecture constraints
- Testing requirements
- API compatibility rules
- Review expectations
`
}

func agentTemplate() string {
	return `# Agent Rules

Before work:

- read task.md
- read instructions.md
- read declaration.md
- read current workspace
- if this is a child task, read the parent task chain before starting work

During work:

- update declaration.md
- do not manually create task folders or files under .tasker/tasks/
- use tasker new, tasker add, or tasker import if a new task is required

After work:

- update result.md
- update status.json

No undocumented changes.
`
}

func configTemplate() string {
	return `editor: ""

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
      show_diff: ["d"]
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
`
}

func workspaceTemplate() string {
	return `# Current Workspace

Current task:

Rules:

Relevant files:

References:

Expected output:
`
}

func filesTemplate() string {
	return `# Relevant Files
`
}

func importTasksTemplate() string {
	return `{
  "_comment_valid_task_types": "bug, decision, documentation, feature, research, review, test",
  "tasks": [
    {
      "_comment_type": "Valid types: bug, decision, documentation, feature, research, review, test",
      "title": "",
      "type": "",
      "body": "",
      "instructions": "",
      "declaration": "",
      "result": "",
      "context": {},
      "subtasks": [
        {
          "_comment_type": "Valid types: bug, decision, documentation, feature, research, review, test",
          "title": "",
          "type": "",
          "body": "",
          "instructions": "",
          "declaration": "",
          "result": "",
          "context": {},
          "subtasks": []
        }
      ]
    }
  ]
}
`
}

func taskDocumentTemplate() string {
	return `# {{TITLE}}

ID: {{ID}}
Type: {{TYPE}}
Created: {{CREATED_AT}}

## Goal

Describe the goal and requirements.
`
}

func taskTypeTemplate(taskType string) string {
	switch taskType {
	case "bug":
		return taskTypeTemplateWithSections("Bug", []taskTemplateSection{
			{Title: "Problem", Prompt: "What is broken?"},
			{Title: "Steps", Prompt: "How can it be reproduced?"},
			{Title: "Expected", Prompt: "What should happen?"},
			{Title: "Notes", Prompt: "Logs, screenshots, or extra context:"},
		})
	case "feature":
		return taskTypeTemplateWithSections("Feature", []taskTemplateSection{
			{Title: "Goal", Prompt: "What are we adding?"},
			{Title: "Details", Prompt: "How should it work?"},
			{Title: "Acceptance", Prompt: "How do we know it is done?"},
		})
	case "research":
		return taskTypeTemplateWithSections("Research", []taskTemplateSection{
			{Title: "Question", Prompt: "What are we trying to understand?"},
			{Title: "Context", Prompt: "Why do we need this?"},
			{Title: "Result", Prompt: "What should the research produce?"},
		})
	case "documentation":
		return taskTypeTemplateWithSections("Documentation", []taskTemplateSection{
			{Title: "Topic", Prompt: "What needs documentation?"},
			{Title: "Audience", Prompt: "Who is this for?"},
			{Title: "Notes", Prompt: "Important details:"},
		})
	case "decision":
		return taskTypeTemplateWithSections("Decision", []taskTemplateSection{
			{Title: "Decision", Prompt: "What needs to be decided?"},
			{Title: "Options", Prompt: "Possible approaches:"},
			{Title: "Notes", Prompt: "Important constraints:"},
		})
	case "review":
		return taskTypeTemplateWithSections("Review", []taskTemplateSection{
			{Title: "Target", Prompt: "What should be reviewed?"},
			{Title: "Focus", Prompt: "What should reviewers check?"},
			{Title: "Notes", Prompt: "Extra context:"},
		})
	case "test":
		return taskTypeTemplateWithSections("Test", []taskTemplateSection{
			{Title: "Target", Prompt: "What behavior or component should be tested?"},
			{Title: "Coverage", Prompt: "What scenarios should the tests cover?"},
			{Title: "Notes", Prompt: "Important setup, fixtures, or constraints:"},
		})
	default:
		return taskDocumentTemplate()
	}
}

type taskTemplateSection struct {
	Title  string
	Prompt string
}

func taskTypeTemplateWithSections(label string, sections []taskTemplateSection) string {
	body := fmt.Sprintf(`# %s

ID: {{ID}}
Type: {{TYPE}}
Created: {{CREATED_AT}}
`, label)

	for _, section := range sections {
		body += fmt.Sprintf(`
## %s

%s

`, section.Title, section.Prompt)
	}

	return body
}
