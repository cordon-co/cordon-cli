#!/usr/bin/env bash
# dev-install.sh — build cordon and install it to a local bin directory for testing
#
# By default installs to ~/.local/bin (created if absent). Override with INSTALL_DIR.
#
# Usage:
#   ./scripts/dev-install.sh
#   INSTALL_DIR=/usr/local/bin ./scripts/dev-install.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR/.."

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BUILD_DIR="build"
BINARY="${BUILD_DIR}/cordon"

echo "Building cordon (dev)..."
go build -ldflags "-X github.com/cordon-co/cordon/cmd.Version=dev" -o "$BINARY" .

mkdir -p "$INSTALL_DIR"
cp "$BINARY" "${INSTALL_DIR}/cordon"
chmod +x "${INSTALL_DIR}/cordon"

echo "Installed: ${INSTALL_DIR}/cordon"

# Remind the user if INSTALL_DIR is not on PATH
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  echo ""
  echo "Note: ${INSTALL_DIR} is not on your PATH."
  echo "Add it with:  export PATH=\"\$PATH:${INSTALL_DIR}\""
fi
