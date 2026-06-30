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
