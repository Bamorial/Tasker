package tasker

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
