#!/usr/bin/env bash
# Smoke tests for llmctl packages.
# Tests installation and basic CLI functionality on Ubuntu 24.04 and Fedora 41
# using Docker.
#
# Usage:
#   bash scripts/smoke.sh
#   SKIP_FEDORA=1 bash scripts/smoke.sh   (only Ubuntu)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${REPO_ROOT}/dist"

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

info()  { printf '\033[0;32m[smoke]\033[0m %s\n' "$*"; }
error() { printf '\033[0;31m[smoke]\033[0m %s\n' "$*" >&2; exit 1; }
pass()  { printf '\033[0;32m[smoke] PASS\033[0m %s\n' "$*"; }
fail()  { printf '\033[0;31m[smoke] FAIL\033[0m %s\n' "$*" >&2; FAILED=1; }

FAILED=0

# ---------------------------------------------------------------------------
# pre-flight
# ---------------------------------------------------------------------------

if ! docker info >/dev/null 2>&1; then
  error "Docker is not running or not accessible."
fi

DEB_PKG="$(ls "${DIST}"/llmctl_*.deb 2>/dev/null | head -1)"
RPM_PKG="$(ls "${DIST}"/llmctl-*.rpm 2>/dev/null | head -1)"

if [ -z "${DEB_PKG}" ]; then
  error "No .deb package found in ${DIST}. Run scripts/build-packages.sh first."
fi

# ---------------------------------------------------------------------------
# Ubuntu 24.04 smoke test
# ---------------------------------------------------------------------------

info "Testing on Ubuntu 24.04..."

if docker run --rm \
  -v "${DIST}:/dist:ro" \
  ubuntu:24.04 bash -c "
    set -euo pipefail
    apt-get update -qq
    apt-get install -y -qq /dist/$(basename '${DEB_PKG}') >/dev/null 2>&1

    echo '--- llmctl --version ---'
    llmctl --version

    echo '--- llmctl config init ---'
    llmctl config init

    echo '--- llmctl model catalog ---'
    llmctl model catalog | grep -q 'Qwen' || { echo 'catalog missing Qwen'; exit 1; }

    echo '--- llmctl server status ---'
    # Server won't be running; just verify the command returns a structured response.
    llmctl server status 2>&1 | grep -q 'status:' || { echo 'server status output unexpected'; exit 1; }

    echo PASS
"; then
  pass "Ubuntu 24.04"
else
  fail "Ubuntu 24.04"
fi

# ---------------------------------------------------------------------------
# Fedora 41 smoke test
# ---------------------------------------------------------------------------

if [ "${SKIP_FEDORA:-0}" != "1" ] && [ -n "${RPM_PKG}" ]; then
  info "Testing on Fedora 41..."

  if docker run --rm \
    -v "${DIST}:/dist:ro" \
    fedora:41 bash -c "
      set -euo pipefail
      dnf install -y -q /dist/$(basename '${RPM_PKG}') >/dev/null 2>&1

      echo '--- llmctl --version ---'
      llmctl --version

      echo '--- llmctl config init ---'
      llmctl config init

      echo '--- llmctl model catalog ---'
      llmctl model catalog | grep -q 'Qwen' || { echo 'catalog missing Qwen'; exit 1; }

      echo '--- llmctl server status ---'
      llmctl server status 2>&1 | grep -q 'status:' || { echo 'server status output unexpected'; exit 1; }

      echo PASS
  "; then
    pass "Fedora 41"
  else
    fail "Fedora 41"
  fi
elif [ -z "${RPM_PKG}" ]; then
  info "No .rpm package found — skipping Fedora smoke test"
fi

# ---------------------------------------------------------------------------
# summary
# ---------------------------------------------------------------------------

if [ "$FAILED" -ne 0 ]; then
  error "One or more smoke tests FAILED"
fi

info "All smoke tests PASSED"
