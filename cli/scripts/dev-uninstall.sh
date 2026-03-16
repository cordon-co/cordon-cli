#!/usr/bin/env bash
# dev-uninstall.sh — remove the cordon binary installed by dev-install.sh
#
# Usage:
#   ./scripts/dev-uninstall.sh
#   INSTALL_DIR=/usr/local/bin ./scripts/dev-uninstall.sh

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BINARY="${INSTALL_DIR}/cordon"

if [ -f "$BINARY" ]; then
    rm "$BINARY"
    echo "Removed: ${BINARY}"
else
    echo "Not found: ${BINARY} (nothing to remove)"
fi
