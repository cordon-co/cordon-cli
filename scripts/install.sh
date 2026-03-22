#!/usr/bin/env sh
# install.sh — download and install the latest cordon binary
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/cordon-co/cordon-cli/main/scripts/install.sh | sh
#
# Environment variables:
#   CORDON_VERSION   — install a specific version (e.g. v0.1.0). Default: latest
#   INSTALL_DIR      — where to place the binary. Default: ~/.local/bin or /usr/local/bin

set -eu

REPO="cordon-co/cordon-cli"
BINARY="cordon"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

log()   { printf '[cordon] %s\n' "$*"; }
error() { printf '[cordon] ERROR: %s\n' "$*" >&2; exit 1; }

need_cmd() {
  if ! command -v "$1" > /dev/null 2>&1; then
    error "need '$1' (command not found)"
  fi
}

# ---------------------------------------------------------------------------
# Detect OS and architecture
# ---------------------------------------------------------------------------

detect_platform() {
  OS="$(uname -s)"
  ARCH="$(uname -m)"

  case "$OS" in
    Linux*)  OS="linux" ;;
    Darwin*) OS="darwin" ;;
    *) error "unsupported OS: $OS (Windows is not currently supported)" ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) error "unsupported architecture: $ARCH" ;;
  esac
}

# ---------------------------------------------------------------------------
# Resolve version
# ---------------------------------------------------------------------------

resolve_version() {
  if [ -n "${CORDON_VERSION:-}" ]; then
    VERSION="$CORDON_VERSION"
  else
    need_cmd curl
    # Follow the redirect from /releases/latest to get the tag name
    VERSION=$(curl -fsSL -o /dev/null -w "%{url_effective}" "https://github.com/${REPO}/releases/latest" | rev | cut -d'/' -f1 | rev)
    if [ -z "$VERSION" ]; then
      error "could not determine latest version"
    fi
  fi
  log "version: ${VERSION}"
}

# ---------------------------------------------------------------------------
# Pick install directory
# ---------------------------------------------------------------------------

pick_install_dir() {
  if [ -n "${INSTALL_DIR:-}" ]; then
    return
  fi

  # Prefer ~/.local/bin if it exists and is on PATH
  if echo "$PATH" | tr ':' '\n' | grep -qx "$HOME/.local/bin" 2>/dev/null; then
    INSTALL_DIR="$HOME/.local/bin"
  elif [ -d "$HOME/.local/bin" ]; then
    INSTALL_DIR="$HOME/.local/bin"
  else
    INSTALL_DIR="/usr/local/bin"
  fi
}

# ---------------------------------------------------------------------------
# Download and install
# ---------------------------------------------------------------------------

install_binary() {
  need_cmd curl
  need_cmd chmod

  ARTIFACT="${BINARY}-${OS}-${ARCH}"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARTIFACT}"

  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT

  log "downloading ${URL}"
  HTTP_CODE=$(curl -fsSL -w "%{http_code}" -o "${TMPDIR}/${ARTIFACT}" "$URL" 2>/dev/null) || true

  if [ ! -f "${TMPDIR}/${ARTIFACT}" ] || [ "${HTTP_CODE:-0}" != "200" ]; then
    error "download failed (HTTP ${HTTP_CODE:-???}). Check that version ${VERSION} exists at https://github.com/${REPO}/releases"
  fi

  chmod +x "${TMPDIR}/${ARTIFACT}"

  # Install — use sudo only if needed
  mkdir -p "$INSTALL_DIR" 2>/dev/null || true
  if [ -w "$INSTALL_DIR" ]; then
    mv "${TMPDIR}/${ARTIFACT}" "${INSTALL_DIR}/${BINARY}"
  else
    log "elevated permissions required to install to ${INSTALL_DIR}"
    sudo mv "${TMPDIR}/${ARTIFACT}" "${INSTALL_DIR}/${BINARY}"
  fi

  log "installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
}

# ---------------------------------------------------------------------------
# Post-install check
# ---------------------------------------------------------------------------

verify_install() {
  if ! command -v "$BINARY" > /dev/null 2>&1; then
    log ""
    log "WARNING: ${INSTALL_DIR} is not in your PATH."
    log "Add it with:"
    log ""
    log "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    log ""
  else
    log "$(${BINARY} version 2>/dev/null || echo "installed successfully")"
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
  log "installing cordon..."
  detect_platform
  log "platform: ${OS}/${ARCH}"
  resolve_version
  pick_install_dir
  install_binary
  verify_install
  log "done!"
}

main
