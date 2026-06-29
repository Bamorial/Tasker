# Releasing Tasker

This repository is prepared to build release binaries, generate Debian packages, and update a Homebrew tap. Public publication is still dependent on external Git hosting, release credentials, and package repository infrastructure.

## What is automated

- Tagged releases run through GoReleaser in [`.github/workflows/release.yml`](../.github/workflows/release.yml).
- Build metadata is embedded into the binary through linker flags and exposed with `tasker version`.
- Release artifacts include:
  - `tasker_<version>_darwin_arm64.tar.gz`
  - `tasker_<version>_linux_amd64.tar.gz`
  - `tasker_<version>_windows_amd64.zip`
  - `tasker_<version>_linux_amd64.deb`
  - `checksums.txt`
- Homebrew formula updates can be created automatically against a separate tap repository.

## Build metadata

Local builds default to:

- `Version=dev`
- `Commit=unknown`
- `Date=<current UTC timestamp>`

Override them explicitly when producing a release build outside GoReleaser:

```bash
VERSION=v0.1.0 COMMIT=$(git rev-parse --short HEAD) DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) ./scripts/build.sh
./bin/tasker version
```

## Local validation

Validate the code and a versioned local build:

```bash
go test ./...
VERSION=v0.1.0 COMMIT=local DATE=2026-06-29T00:00:00Z ./scripts/build.sh
./bin/tasker version
```

If GoReleaser is installed locally, validate the packaging config without publishing:

```bash
goreleaser release --snapshot --clean
```

This should emit archives and a `.deb` package under `dist/`.

## GitHub release flow

The release workflow expects:

- a real Git repository with tags
- GitHub Releases enabled
- `HOMEBREW_TAP_OWNER` repository variable
- `HOMEBREW_TAP_NAME` repository variable
- `HOMEBREW_TAP_GITHUB_TOKEN` secret if the tap lives in a different repository

Release steps:

1. Ensure `go test ./...` passes.
2. Tag the release as `vX.Y.Z`.
3. Push the tag.
4. Let GitHub Actions run the `release` workflow.
5. Verify the GitHub Release contains archives, checksums, and the `.deb`.

## Homebrew

Homebrew distribution is designed around a separate tap repository such as `bamorial/homebrew-tap`.

The GoReleaser `brews` section will:

- generate `Formula/tasker.rb`
- update the tap repository
- point the formula to the tagged GitHub Release archive and checksum

Maintainer setup:

1. Create the tap repository.
2. Add `HOMEBREW_TAP_OWNER` and `HOMEBREW_TAP_NAME` to the source repository variables.
3. Add `HOMEBREW_TAP_GITHUB_TOKEN` if the default `GITHUB_TOKEN` cannot write to the tap.
4. Trigger a tagged release.

Consumer install path:

```bash
brew tap <owner>/<tap-repo>
brew install tasker
```

If automated tap updates are unavailable, generate a snapshot release and update the formula in the tap repo manually from the emitted archive URL and checksum.

## Debian and `apt`

GoReleaser uses NFPM to produce the `.deb` package. Hosting that package behind `apt install tasker` still requires a Debian repository.

Recommended hosting approach: use `aptly` to publish a signed repository to S3, static hosting, or another HTTP origin.

Suggested repository layout:

- distribution: `stable`
- component: `main`
- architecture: `amd64`

Example maintainership flow with `aptly`:

```bash
aptly repo create -distribution=stable -component=main tasker
aptly repo add tasker dist/*.deb
aptly publish repo -architectures=amd64 tasker
```

Use [`packaging/apt/aptly.conf.example`](../packaging/apt/aptly.conf.example) as a starting point for local configuration. Once the repository is published and signed, consumers can install with:

```bash
sudo apt update
sudo apt install tasker
```

## Current assumptions

- The module path remains `github.com/bamorial/tasker`.
- Releases are cut from Git tags in GitHub Actions.
- Debian publishing infrastructure is external to this repository.
- Homebrew uses a tap repository rather than Homebrew/homebrew-core.
