#!/usr/bin/env bash
#
# Build and install Discord's libdave, which godave/golibdave dynamically links
# against. The install script ships inside the godave module, so we locate the
# module directory via `go list -m` rather than hardcoding a GOPATH path.
#
# Usage: ./scripts/install-libdave.sh [version]
#        version defaults to v1.1.0
#
set -euo pipefail

VERSION="${1:-v1.1.0}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go not found in PATH" >&2
  exit 1
fi

echo "==> resolving godave module directory"
GOdave_DIR="$(go list -m -f '{{.Dir}}' github.com/disgoorg/godave 2>/dev/null || true)"

if [ -z "${GOdave_DIR}" ] || [ ! -d "${GOdave_DIR}" ]; then
  echo "==> godave not in module cache yet, downloading"
  go mod download github.com/disgoorg/godave
  GOdave_DIR="$(go list -m -f '{{.Dir}}' github.com/disgoorg/godave)"
fi

if [ -z "${GOdave_DIR}" ] || [ ! -d "${GOdave_DIR}" ]; then
  echo "error: could not locate godave module directory" >&2
  exit 1
fi

echo "==> godave at ${GOdave_DIR}"

if [ -x "${GOdave_DIR}/scripts/libdave_install.sh" ]; then
  INSTALLER="${GOdave_DIR}/scripts/libdave_install.sh"
elif [ -x "${GOdave_DIR}/libdave_install.sh" ]; then
  INSTALLER="${GOdave_DIR}/libdave_install.sh"
else
  echo "error: no libdave install script found under ${GOdave_DIR}" >&2
  exit 1
fi

echo "==> running ${INSTALLER} ${VERSION}"
cd "${GOdave_DIR}"
"./${INSTALLER#"${GOdave_DIR}/"}" "${VERSION}"

echo
echo "==> libdave ${VERSION} installed."
echo "    Linux: run 'sudo ldconfig' so the loader picks it up, or set"
echo "    LD_LIBRARY_PATH=/usr/local/lib:\$LD_LIBRARY_PATH"
