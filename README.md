# Tasker

Tasker is a CLI-first universal agent workspace protocol. The idea is simple: humans need documents to stay organized, and agents apparently need their own little paper trail too.

 This project, was also done using Tasker.

## Current V1 Scope

Implemented:

- `tasker init`
- `tasker checkout <id>`
- `tasker instruction <id>`
- `tasker instructions`
- `tasker new [title]`
- `tasker add [title]`
- `tasker meta <id>`
- `tasker delete <id>`
- `tasker open <id>`
- `tasker tree`
- `tasker status`
- `tasker status <id>`
- `tasker version`


## Build

Requirements:

- Go 1.22+

Commands:

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

## Notes

- Task IDs are global across the workspace.
- `tasker instruction <id>` opens a task's `instructions.md`.
- `tasker status` shows a workspace-wide task tree, including subtasks plus per-task status metadata such as type, agent, and child counts.
- `tasker status <id>` shows details for one task plus its subtasks and handoff notes.
- `tasker status` uses ANSI color and stronger section formatting when writing to an interactive terminal, and falls back to plain text when redirected.
- `tasker checkout <id>` populates `.tasker/current/WORKSPACE.md`, `.tasker/current/FILES.md`, and `.tasker/current/CONTEXT.json` so the next agent has explicit workspace context.
- When Git is enabled, `tasker checkout <id>` only auto-creates or reuses a branch if `git.checkout_branch: true` is set in `.tasker/config.yaml`.
- `tasker checkout <id> --existing-branch <name>` links a task to an existing branch, and `--branch <name>` lets you override the generated task branch name.
- Agent guidance expects child-task work to include reading the parent task chain for inherited context and constraints.
- Child tasks are stored in each task's `children/` directory.
- `tasker delete <id>` removes a leaf task, and requires `--recursive` for tasks that have children.
- `tasker new` and `tasker add` support `--open task|instructions|declaration|result|meta` and `--no-open`.
- `tasker add` accepts `--parent`, but can also infer the parent from the current task directory or `.tasker/current/CONTEXT.json`.
- `tasker meta <id>` opens `meta.json` by default, and can apply validated updates with `--title` and `--type`.
- Editor resolution now checks `.tasker/config.yaml` first, then falls back to `$EDITOR`.

Example config:

```yaml
editor: "code -w"

git:
  enabled: false
  branch_per_task: true
  checkout_branch: false
  commit_per_subtask: true
  branch_prefix: "task"
```

Global install:

- `./scripts/install.sh` builds `bin/tasker` and symlinks it to `/opt/homebrew/bin/tasker`
- override the target with `TASKER_INSTALL_DIR=/some/path ./scripts/install.sh`

## Packaging

- Tagged releases are configured through GoReleaser in `.goreleaser.yaml`
- GitHub Actions release automation lives in `.github/workflows/release.yml`
- Debian package generation is configured through GoReleaser NFPM
- Maintainer release workflow is documented in `docs/releasing.md`
