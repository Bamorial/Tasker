This repository uses Tasker.

Before working:

1. Read .tasker/START.md
2. Read .tasker/current/WORKSPACE.md if it exists
3. Read .tasker/instructions.md
4. Read the current task folder, and if the task is a child task, read its parent task chain as well

All important communication must happen through:

- declaration.md
- result.md
- status.json

Never create task folders or files manually under .tasker/tasks. Use `tasker new`, `tasker add`, or `tasker import`.

Never leave undocumented changes.
