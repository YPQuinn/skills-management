#!/usr/bin/env bash
# Run README quickstart commands in an isolated HOME when README.md exists.
# The next ticket owns README content; this harness must not invent one.
set -euo pipefail

root=$(cd -P "$(dirname "$0")/../.." && pwd)
cd "$root"

if [ ! -f "$root/README.md" ]; then
  echo "README.md is missing; README quickstart smoke fails closed" >&2
  exit 1
fi

bin=${SKILLCTL_BIN:-$root/dist/skillctl}
if [ ! -x "$bin" ]; then
  echo "skillctl binary not found at $bin; run ./scripts/build.sh first" >&2
  exit 1
fi

# Extract fenced shell blocks and keep only skillctl / PATH-style quickstart lines.
mapfile -t commands < <(awk '
  BEGIN { in_block = 0 }
  /^```(bash|sh|shell|zsh)?[[:space:]]*$/ { in_block = 1; next }
  /^```/ { in_block = 0; next }
  in_block && $0 ~ /^[[:space:]]*(skillctl|\$[[:space:]]*skillctl)/ {
    sub(/^[[:space:]]*\$[[:space:]]*/, "")
    print
  }
' "$root/README.md")

if [ "${#commands[@]}" -eq 0 ]; then
  echo "README.md exists but contains no skillctl quickstart commands" >&2
  exit 1
fi

home=$(mktemp -d)
trap 'rm -rf "$home"' EXIT
export HOME=$home
export PATH="$(dirname "$bin"):$PATH"

echo "==> README quickstart in isolated HOME $home"
for cmd in "${commands[@]}"; do
  echo "+ $cmd"
  # Reject anything that is not a skillctl invocation.
  case $cmd in
    skillctl*) ;;
    *)
      echo "refusing non-skillctl README command: $cmd" >&2
      exit 1
      ;;
  esac
  # shellcheck disable=SC2086
  eval "$cmd"
done

echo "README quickstart smoke passed"
