#!/usr/bin/env bash
# llmctl installer
# Usage:
#   curl -sSf https://github.com/mwigge/llmctl/releases/latest/download/install.sh | bash
#   OFFLINE=1 ./install.sh   (uses ./llmctl binary in current dir)
set -euo pipefail

REPO="mwigge/llmctl"
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="llmctl"
VERSION="${VERSION:-latest}"

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

info()  { printf '\033[0;32m[llmctl]\033[0m %s\n' "$*"; }
warn()  { printf '\033[0;33m[llmctl]\033[0m %s\n' "$*" >&2; }
error() { printf '\033[0;31m[llmctl]\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# OS / arch detection
# ---------------------------------------------------------------------------

OS="$(uname -s)"
ARCH="$(uname -m)"

if [ "$OS" != "Linux" ]; then
  error "This installer only supports Linux.
On macOS, install via Homebrew (coming soon) or build from source:
  git clone https://github.com/${REPO}
  cd llmctl && CGO_ENABLED=1 go build -o ~/.local/bin/llmctl ./cmd/llmctl"
fi

case "$ARCH" in
  x86_64)  ARCH_SLUG="amd64" ;;
  aarch64) ARCH_SLUG="arm64" ;;
  arm64)   ARCH_SLUG="arm64" ;;
  *)
    error "Unsupported architecture: ${ARCH}. Supported: x86_64, aarch64."
    ;;
esac

info "Detected: Linux / ${ARCH} (${ARCH_SLUG})"

# ---------------------------------------------------------------------------
# install directory
# ---------------------------------------------------------------------------

mkdir -p "${INSTALL_DIR}"

# Ensure INSTALL_DIR is on PATH (idempotent).
if ! echo "$PATH" | grep -q "${INSTALL_DIR}"; then
  warn "${INSTALL_DIR} is not in PATH. Add it to your shell profile:"
  warn "  export PATH=\"\${HOME}/.local/bin:\${PATH}\""
fi

# ---------------------------------------------------------------------------
# obtain binary
# ---------------------------------------------------------------------------

DEST="${INSTALL_DIR}/${BINARY_NAME}"

if [ "${OFFLINE:-0}" = "1" ]; then
  info "Offline mode: using local binary"
  if [ ! -f "./llmctl" ]; then
    error "OFFLINE=1 but ./llmctl not found in current directory"
  fi
  cp ./llmctl "${DEST}"
  chmod +x "${DEST}"
else
  if [ "$VERSION" = "latest" ]; then
    DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/llmctl-linux-${ARCH_SLUG}"
  else
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/llmctl-linux-${ARCH_SLUG}"
  fi

  info "Downloading ${BINARY_NAME} from ${DOWNLOAD_URL} ..."

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "${DEST}.tmp" "${DOWNLOAD_URL}"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "${DEST}.tmp" "${DOWNLOAD_URL}"
  else
    error "Neither curl nor wget found. Install one of them and retry."
  fi

  chmod +x "${DEST}.tmp"
  mv "${DEST}.tmp" "${DEST}"
fi

info "Installed ${BINARY_NAME} to ${DEST}"

# ---------------------------------------------------------------------------
# initialise default config
# ---------------------------------------------------------------------------

info "Initialising default configuration..."
"${DEST}" config init 2>/dev/null || true

# Refresh services on install, upgrade, or reinstall. This keeps the local
# server using the freshly installed llmctl binary and updated config handling.
if command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload >/dev/null 2>&1 || true
  if systemctl --user list-unit-files llmctl-server.service >/dev/null 2>&1; then
    info "Restarting llmctl local server service..."
    systemctl --user enable --now llmctl-server.service >/dev/null 2>&1 || true
    systemctl --user restart llmctl-server.service >/dev/null 2>&1 || true
  fi
fi

# ---------------------------------------------------------------------------
# next steps
# ---------------------------------------------------------------------------

cat <<'EOF'

llmctl is ready. Next steps:

  1. Download a model:
       llmctl model install Hermes-3-Llama-3.1-8B

  2. Install the llama.cpp server:
       llmctl server install

     Or install with GPU-aware model selection:
       llmctl server install-gpu

  3. Start the server:
       llmctl server start

  4. Point any OpenAI-compatible client at:
       http://localhost:8765/v1

  Run 'llmctl --help' for full usage.
EOF
