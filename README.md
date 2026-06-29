# Tasker

Tasker is a CLI-first universal agent workspace protocol. The idea is simple: humans need documents to stay organized, and agents apparently need their own little paper trail too.

 This project, was also done using Tasker.

## Current V1 Scope

Implemented:

- `tasker init`
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
- `tasker status` shows a workspace-wide task overview, and `tasker status <id>` shows details for one task plus its subtasks and handoff notes.
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
  commit_per_subtask: true
```

Global install:

- `./scripts/install.sh` builds `bin/tasker` and symlinks it to `/opt/homebrew/bin/tasker`
- override the target with `TASKER_INSTALL_DIR=/some/path ./scripts/install.sh`

## Packaging

- Tagged releases are configured through GoReleaser in `.goreleaser.yaml`
- GitHub Actions release automation lives in `.github/workflows/release.yml`
- Debian package generation is configured through GoReleaser NFPM
- Maintainer release workflow is documented in `docs/releasing.md`
