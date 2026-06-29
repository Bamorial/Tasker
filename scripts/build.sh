#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${1:-$REPO_ROOT/bin}"
OUTPUT_BIN="$OUTPUT_DIR/tasker"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-unknown}"
DATE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-X github.com/bamorial/tasker/internal/buildinfo.Version=${VERSION} -X github.com/bamorial/tasker/internal/buildinfo.Commit=${COMMIT} -X github.com/bamorial/tasker/internal/buildinfo.Date=${DATE}"

if ! command -v go >/dev/null 2>&1; then
  echo "go is not installed or not in PATH" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

go -C "$REPO_ROOT" mod tidy
go -C "$REPO_ROOT" build -ldflags "$LDFLAGS" -o "$OUTPUT_BIN" .

echo "Built $OUTPUT_BIN"
