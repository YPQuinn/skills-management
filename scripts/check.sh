#!/usr/bin/env bash
# Stable static, unit, and integration checks from decision 09.
# Frontend lint is the repo's Oxlint configuration; ESLint is not used.
set -euo pipefail

# Physical path: macOS /var is a symlink to /private/var; Go filepath
# resolves the real path, so a logical cwd breaks NormalizeLocal.
root=$(cd -P "$(dirname "$0")/.." && pwd)
cd "$root"

webui="$root/internal/webui"

echo "==> pnpm install"
(cd "$webui" && pnpm install --frozen-lockfile)

echo "==> TypeScript"
(cd "$webui" && pnpm exec tsc -b --pretty false)

echo "==> Oxlint"
(cd "$webui" && pnpm run lint)

echo "==> Vitest"
(cd "$webui" && pnpm run test)

echo "==> fresh Vite dist"
rm -rf "$webui/dist"
(cd "$webui" && pnpm exec vite build)
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

# GO_TEST_FLAGS lets CI run the suite once under -race instead of twice.
echo "==> go test${GO_TEST_FLAGS:+ ${GO_TEST_FLAGS}}"
go test ${GO_TEST_FLAGS:-} ./...

echo "checks passed"
