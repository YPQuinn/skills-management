#!/usr/bin/env bash
# Run README fenced skillctl quickstart commands in an isolated HOME.
# Bash 3.2 compatible. Commands are exec'd as argv; eval is never used.
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
commands=()
while IFS= read -r line; do
  commands+=("$line")
done < <(awk '
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
cd "$home"

# Self-contained local Source and Target fixtures for the README workflow.
mkdir -p "$home/my-skills/demo-skill"
cat >"$home/my-skills/demo-skill/SKILL.md" <<'EOF'
---
name: demo-skill
description: README smoke fixture
---
# Demo Skill
EOF
mkdir -p "$home/agent-skills"

echo "==> README quickstart in isolated HOME $home"

# Refuse shell metacharacters and command substitution. Tokens are split on
# IFS whitespace only; quoting is not supported because the README workflow
# never needs it. noglob so unquoted splitting cannot expand files.
run_skillctl_line() {
  line=$1
  case $line in
    skillctl*) ;;
    *)
      echo "refusing non-skillctl README command: $line" >&2
      exit 1
      ;;
  esac
  case $line in
    "skillctl ui" | "skillctl ui "*)
      echo "refusing hanging ui command: $line" >&2
      exit 1
      ;;
  esac
  case $line in
    *'$'* | *'`'* | *'|'* | *'&'* | *';'* | *'<'* | *'>'* | *'('* | *')'* | *'{'* | *'}'* | *'*'* | *'?'* | *'['* | *']'* | *'#'* | *'!'* | *'\\'*)
      echo "refusing shell metacharacters or command substitution: $line" >&2
      exit 1
      ;;
  esac
  case $line in
    *\"* | *"'"*)
      echo "refusing shell metacharacters or command substitution: $line" >&2
      exit 1
      ;;
  esac
  set -f
  # shellcheck disable=SC2086
  set -- $line
  set +f
  if [ "$#" -lt 1 ] || [ "$1" != "skillctl" ]; then
    echo "refusing unparseable skillctl command: $line" >&2
    exit 1
  fi
  shift
  "$bin" "$@"
}

for cmd in "${commands[@]}"; do
  echo "+ $cmd"
  run_skillctl_line "$cmd"
done

link=$home/agent-skills/demo-skill
if [ ! -L "$link" ]; then
  echo "distribution did not create Managed Link $link" >&2
  exit 1
fi

echo "README quickstart smoke passed"
