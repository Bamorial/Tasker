#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$REPO_ROOT/bin"
OUTPUT_BIN="$BUILD_DIR/tasker"
INSTALL_DIR="${TASKER_INSTALL_DIR:-/opt/homebrew/bin}"
INSTALL_PATH="$INSTALL_DIR/tasker"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-unknown}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-X github.com/bamorial/tasker/internal/buildinfo.Version=${VERSION} -X github.com/bamorial/tasker/internal/buildinfo.Commit=${COMMIT} -X github.com/bamorial/tasker/internal/buildinfo.Date=${DATE}"

if ! command -v go >/dev/null 2>&1; then
  echo "go is not installed or not in PATH" >&2
  exit 1
fi

mkdir -p "$BUILD_DIR"

echo "Building tasker..."
go -C "$REPO_ROOT" mod tidy
go -C "$REPO_ROOT" build -ldflags "$LDFLAGS" -o "$OUTPUT_BIN" .

echo "Installing to $INSTALL_PATH"
mkdir -p "$INSTALL_DIR"
ln -sf "$OUTPUT_BIN" "$INSTALL_PATH"

echo "Installed:"
echo "  Binary:  $OUTPUT_BIN"
echo "  Command: $INSTALL_PATH"
echo
"$INSTALL_PATH" --help
