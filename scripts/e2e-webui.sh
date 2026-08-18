#!/usr/bin/env bash
# Build the embedded skillctl binary and run Playwright against it.
set -euo pipefail

root=$(cd -P "$(dirname "$0")/.." && pwd)
cd "$root"

"$root/scripts/build.sh"
(
  cd "$root/e2e/webui"
  npm ci
  if [ -n "${CI:-}" ] && [ "$(uname -s)" = Linux ]; then
    npx playwright install --with-deps
  else
    npx playwright install
  fi
  npx playwright test "$@"
)
