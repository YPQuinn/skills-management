#!/usr/bin/env bash
# Install locked frontend dependencies, generate a fresh Appica dist, and
# build the native development binary at dist/skillctl.
set -euo pipefail

# Physical path: macOS /var is a symlink to /private/var; Go filepath
# resolves the real path, so a logical cwd breaks NormalizeLocal.
root=$(cd -P "$(dirname "$0")/.." && pwd)
cd "$root"

webui="$root/internal/webui"
version=dev
commit=unknown
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  commit=$(git rev-parse HEAD)
fi
ldflags="-X skillctl/internal/cli.version=${version} -X skillctl/internal/cli.gitCommit=${commit}"

echo "==> npm ci"
(cd "$webui" && npm ci)

echo "==> fresh Vite dist"
rm -rf "$webui/dist"
(cd "$webui" && npm run build)
if [ ! -f "$webui/dist/index.html" ]; then
  echo "frontend build did not produce internal/webui/dist/index.html" >&2
  exit 1
fi

echo "==> go build dist/skillctl"
mkdir -p "$root/dist"
go build -ldflags "$ldflags" -o "$root/dist/skillctl" ./cmd/skillctl

echo "built dist/skillctl ${version} (${commit})"
