#!/usr/bin/env bash
# Stable static, unit, and integration checks from decision 09.
# Frontend lint is the repo's Oxlint configuration; ESLint is not used.
set -euo pipefail

# Physical path: macOS /var is a symlink to /private/var; Go filepath
# resolves the real path, so a logical cwd breaks NormalizeLocal.
root=$(cd -P "$(dirname "$0")/.." && pwd)
cd "$root"

webui="$root/internal/webui"

echo "==> npm ci"
(cd "$webui" && npm ci)

echo "==> TypeScript"
(cd "$webui" && npx tsc -b --pretty false)

echo "==> Oxlint"
(cd "$webui" && npm run lint)

echo "==> Vitest"
(cd "$webui" && npm run test)

echo "==> fresh Vite dist"
rm -rf "$webui/dist"
(cd "$webui" && npx vite build)
if [ ! -f "$webui/dist/index.html" ]; then
  echo "frontend build did not produce internal/webui/dist/index.html" >&2
  exit 1
fi

echo "==> gofmt"
unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
  echo "gofmt needed:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./...

echo "checks passed"
