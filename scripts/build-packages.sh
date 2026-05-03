#!/usr/bin/env bash
# Build .deb and .rpm packages for llmctl using fpm inside a Docker container.
# Mirrors the pattern from milliways scripts/build-linux-amd64.sh.
#
# Prerequisites:
#   - Docker with buildx support
#   - The llmctl binary must already be built as ./dist/llmctl
#
# Usage:
#   VERSION=v0.1.0 bash scripts/build-packages.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${REPO_ROOT}/dist"
VERSION="${VERSION:-$(git -C "${REPO_ROOT}" describe --tags --always 2>/dev/null || echo 'dev')}"
PKG_VERSION="${VERSION#v}"  # strip leading 'v' for package version

DOCKER_IMAGE="milliways-build-linux:bookworm"

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

info()  { printf '\033[0;32m[build-packages]\033[0m %s\n' "$*"; }
error() { printf '\033[0;31m[build-packages]\033[0m %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# pre-flight
# ---------------------------------------------------------------------------

if [ ! -f "${DIST}/llmctl" ]; then
  error "Binary not found at ${DIST}/llmctl. Build it first:
  CGO_ENABLED=1 go build -o dist/llmctl ./cmd/llmctl"
fi

if ! docker info >/dev/null 2>&1; then
  error "Docker is not running or not accessible."
fi

mkdir -p "${DIST}"

# ---------------------------------------------------------------------------
# build packaging image if not present
# ---------------------------------------------------------------------------

if ! docker image inspect "${DOCKER_IMAGE}" >/dev/null 2>&1; then
  info "Building ${DOCKER_IMAGE}..."
  docker build -f - -t "${DOCKER_IMAGE}" <<'DOCKERFILE'
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ruby ruby-dev build-essential rpm curl ca-certificates \
    && gem install fpm --no-document \
    && rm -rf /var/lib/apt/lists/*
DOCKERFILE
fi

# ---------------------------------------------------------------------------
# staging: build the file layout that fpm will package
# ---------------------------------------------------------------------------

STAGING="$(mktemp -d)"
trap 'rm -rf "$STAGING"' EXIT

# Binary
install -Dm755 "${DIST}/llmctl" "${STAGING}/usr/bin/llmctl"

# Install script (shared data)
install -Dm755 "${REPO_ROOT}/scripts/install.sh" \
  "${STAGING}/usr/share/llmctl/scripts/install.sh"

# systemd user unit template
UNIT_DIR="${STAGING}/usr/lib/systemd/user"
mkdir -p "${UNIT_DIR}"
cat > "${UNIT_DIR}/llmctl-server.service" <<UNIT
[Unit]
Description=llmctl local LLM server
After=network.target

[Service]
Type=simple
ExecStart=${HOME}/.local/bin/llmctl server start --foreground
Restart=on-failure
RestartSec=5s
Environment=HOME=%h

[Install]
WantedBy=default.target
UNIT

# Post-install script (runs after package install on the target system)
POSTINSTALL="$(mktemp)"
trap 'rm -f "$POSTINSTALL"; rm -rf "$STAGING"' EXIT
cat > "${POSTINSTALL}" <<'POSTSCRIPT'
#!/bin/sh
set -e
# Initialise system-wide config if not already present.
if [ ! -f /etc/llmctl/config.yaml ]; then
  llmctl config init --system 2>/dev/null || true
fi
POSTSCRIPT
chmod +x "${POSTINSTALL}"

# ---------------------------------------------------------------------------
# run fpm inside the container
# ---------------------------------------------------------------------------

COMMON_ARGS=(
  fpm
  --input-type dir
  --version "${PKG_VERSION}"
  --name llmctl
  --description "Local LLM management — install, configure, and run llama.cpp models"
  --url "https://github.com/mwigge/llmctl"
  --maintainer "Morgan Wigge <morgan@wigge.nu>"
  --license MIT
  --after-install /work/postinstall.sh
  --chdir "${STAGING}"
  .
)

info "Building .deb package (version ${PKG_VERSION})..."
docker run --rm \
  -v "${STAGING}:/pkg" \
  -v "${DIST}:/dist" \
  -v "${POSTINSTALL}:/work/postinstall.sh:ro" \
  -w /pkg \
  "${DOCKER_IMAGE}" \
  fpm \
    --input-type dir \
    --output-type deb \
    --version "${PKG_VERSION}" \
    --name llmctl \
    --description "Local LLM management — install, configure, and run llama.cpp models" \
    --url "https://github.com/mwigge/llmctl" \
    --maintainer "Morgan Wigge <morgan@wigge.nu>" \
    --license MIT \
    --after-install /work/postinstall.sh \
    --package "/dist/llmctl_${PKG_VERSION}_amd64.deb" \
    .

info "Building .rpm package (version ${PKG_VERSION})..."
docker run --rm \
  -v "${STAGING}:/pkg" \
  -v "${DIST}:/dist" \
  -v "${POSTINSTALL}:/work/postinstall.sh:ro" \
  -w /pkg \
  "${DOCKER_IMAGE}" \
  fpm \
    --input-type dir \
    --output-type rpm \
    --version "${PKG_VERSION}" \
    --name llmctl \
    --description "Local LLM management — install, configure, and run llama.cpp models" \
    --url "https://github.com/mwigge/llmctl" \
    --maintainer "Morgan Wigge <morgan@wigge.nu>" \
    --license MIT \
    --after-install /work/postinstall.sh \
    --package "/dist/llmctl-${PKG_VERSION}-1.x86_64.rpm" \
    .

info "Packages written to ${DIST}:"
ls -lh "${DIST}/"*.deb "${DIST}/"*.rpm 2>/dev/null || true
