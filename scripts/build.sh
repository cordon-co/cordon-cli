#!/usr/bin/env bash
# build.sh — build cordon for the current platform
#
# Usage:
#   ./scripts/build.sh              # build for current OS/arch → build/cordon
#   ./scripts/build.sh all          # cross-compile all release targets
#   VERSION=1.2.3 ./scripts/build.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/../cli"

VERSION="${VERSION:-dev}"
BUILD_DIR="build"
LDFLAGS="-X github.com/cordon-co/cordon/cmd.Version=${VERSION}"

mkdir -p "$BUILD_DIR"

build_target() {
  local os="$1" arch="$2" ext="${3:-}"
  local out="${BUILD_DIR}/cordon-${os}-${arch}${ext}"
  echo "  building ${out}..."
  GOOS="$os" GOARCH="$arch" go build -ldflags "$LDFLAGS" -o "$out" .
}

if [[ "${1:-}" == "all" ]]; then
  echo "Building all targets (version: ${VERSION})"
  build_target darwin  arm64
  build_target darwin  amd64
  build_target linux   amd64
  build_target linux   arm64
  build_target windows amd64 ".exe"
  echo "Done. Artifacts in ${BUILD_DIR}/"
else
  echo "Building for current platform (version: ${VERSION})"
  go build -ldflags "$LDFLAGS" -o "${BUILD_DIR}/cordon" .
  echo "Done. Binary at ${BUILD_DIR}/cordon"
fi
