# Git Worktree Plan

## Why this fits Tasker

Tasker already models work as task folders with stable IDs, handoff docs, and status files. That makes it a good orchestration layer for Git worktrees:

- task ID gives a stable branch/worktree key
- task metadata gives a human-readable branch slug
- task status and declaration files explain why a worktree exists
- multiple terminals or agents can target different task IDs without guessing paths

The right role for Tasker is not "replace Git". It should be the task-aware control plane above Git.

## Important constraints

### 1. A CLI cannot `cd` your current shell

`tasker checkout 001` cannot directly move an already-open terminal into another directory. The command must either:

- print the target path so a shell wrapper can `cd` into it
- print shell exports for `eval`

Recommended shell helper:

```bash
tasker-enter() {
  local dir
  dir="$(tasker checkout "$1" --print-path "${@:2}")" || return
  cd "$dir" || return
}
```

### 2. Worktree runtime state should not live only in tracked `.tasker` files

Each Git worktree gets its own checked-out copy of `.tasker/`. If Tasker stores live worktree registration only in tracked files, the tracker state will fragment across branches.

Recommended split:

- tracked, branch-local state stays in `.tasker/tasks/...`
- shared runtime state lives in the Git common directory, for example `.git/tasker/`

Use `git rev-parse --git-common-dir` so every worktree sees the same registry.

### 3. Existing branches have a Git safety rule

Git does not like the same branch being checked out in multiple worktrees at once. Linking an existing branch should be supported, but only when that branch is not already checked out elsewhere, unless the user explicitly forces it.

Safer default:

- for parallel work, create a new task branch from a base branch
- allow linking an existing branch only when it is free

## Recommended user workflow

### New task branch and worktree

1. User creates or picks a task, for example `006`.
2. User runs `tasker-enter 006`.
3. Tasker creates:
   - branch: `task/006-add-git-worktree-implementation-for-multiple-agent-work`
   - worktree path: configurable root + `006-add-git-worktree-implementation-for-multiple-agent-work`
4. Terminal lands in that worktree.
5. Tasker records the branch, path, base branch, and timestamps in shared runtime state.

Git operation behind it:

```bash
git worktree add -b task/006-add-git-worktree-implementation-for-multiple-agent-work <path> main
```

### Link an existing branch

1. User runs `tasker-enter 006 --existing-branch feature/auth`.
2. Tasker checks whether `feature/auth` is already checked out in another worktree.
3. If free, Tasker creates a worktree for that branch and links task `006` to it.
4. If occupied, Tasker returns a clear error and suggests creating a task branch from it instead.

Git operation behind it:

```bash
git worktree add <path> feature/auth
```

### Reuse an existing task worktree

If task `006` already has a registered worktree, `tasker checkout 006` should return the existing path by default instead of creating another one.

### Close a task worktree

When work is done:

1. User runs `tasker worktree remove 006`
2. Tasker verifies the worktree is clean, or requires `--force`
3. Tasker removes the worktree
4. Tasker optionally keeps the branch unless `--delete-branch` is passed
5. Tasker removes the runtime registry entry

## Recommended CLI surface

Do not overload `tasker open`; it already means "open a document in the editor".

Add new commands:

```text
tasker checkout <id> [--base main] [--branch <name>] [--existing-branch <name>] [--print-path]
tasker worktree list
tasker worktree status [id]
tasker worktree remove <id> [--force] [--delete-branch]
tasker worktree prune
```

Behavior:

- `checkout` is the task-aware entry point
- `worktree list` shows all registered task worktrees
- `worktree status 006` shows branch, path, base, and cleanliness
- `remove` cleans up one task worktree
- `prune` reconciles Tasker runtime state with `git worktree list --porcelain`

## Config changes

Expand `.tasker/config.yaml`:

```yaml
git:
  enabled: true
  default_base_branch: main
  worktree_root: ".tasker/worktrees"
  branch_prefix: "task"
  reuse_existing_worktree: true
  allow_existing_branch_link: true
```

Notes:

- `worktree_root` should be configurable because some users will prefer a sibling directory outside the repo
- defaulting to `main` is fine, but Tasker should also be able to detect the current branch or remote HEAD when unset

## Shared runtime state

Store non-committed orchestration data in the Git common dir:

```text
.git/tasker/worktrees.json
.git/tasker/tasks/006.json
```

Suggested record:

```json
{
  "task_id": "006",
  "task_slug": "add-git-worktree-implementation-for-multiple-agent-work",
  "branch": "task/006-add-git-worktree-implementation-for-multiple-agent-work",
  "base_branch": "main",
  "worktree_path": "/abs/path/to/worktree",
  "mode": "new-branch",
  "created_at": "2026-06-29T19:32:50+03:00",
  "updated_at": "2026-06-29T19:32:50+03:00"
}
```

This should be the source of truth for live task-to-worktree mapping.

## Code integration points in this repo

Current repo state:

- `cmd/open.go` is editor-oriented and should stay that way
- `cmd/root.go` is where new top-level commands get registered
- `internal/tasker/config.go` already has a `GitConfig`, but it is only a stub
- `internal/tasker/tasks.go` already exposes task IDs, slugs, and task lookup needed for branch naming

Recommended additions:

- `cmd/checkout.go`
- `cmd/worktree.go`
- `internal/tasker/git.go`
- `internal/tasker/worktrees.go`

Responsibilities:

- `git.go`: Git command execution, repo discovery, branch/worktree inspection
- `worktrees.go`: Tasker runtime registry, naming, validation, reconciliation
- command files: CLI flags and user-facing output

## Status output changes

Update `tasker status <id>` to include Git info when a worktree is registered:

```text
Git branch: task/006-add-git-worktree-implementation-for-multiple-agent-work
Worktree: /abs/path/to/worktree
Base: main
```

This keeps the tracker useful from the main terminal without needing to inspect Git manually.

## Implementation order

### Phase 1: core checkout

1. Extend config parsing for real Git/worktree settings.
2. Add repo discovery and shared Git common-dir runtime storage.
3. Implement `tasker checkout <id>` for new-branch worktrees.
4. Add `--print-path` for shell integration.
5. Add tests for branch naming, registry writes, and idempotent checkout reuse.

### Phase 2: existing branch support

1. Add `--existing-branch`.
2. Detect whether that branch is already checked out elsewhere.
3. Return a safe error or require explicit force.
4. Record the linked branch in runtime state.

### Phase 3: visibility and cleanup

1. Add `tasker worktree list`
2. Add `tasker worktree status [id]`
3. Add `tasker worktree remove`
4. Add `tasker worktree prune`
5. Surface branch/worktree info in `tasker status <id>`

## Bottom line

The best workflow is:

- keep Tasker as the task tracker
- use one worktree per active task
- create a fresh task branch by default
- allow existing-branch linking only with Git-aware safety checks
- store live worktree registry in the shared Git common dir, not only in tracked `.tasker` files
- use a shell wrapper so `tasker checkout` can effectively move a terminal into the right worktree
