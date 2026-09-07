#!/usr/bin/env bash
# Cold-cache Git add + import timing against mattpocock/skills.
# Usage: measure.sh <label> [skillctl-bin]
set -euo pipefail

root=$(cd -P "$(dirname "$0")/../../.." && pwd)
label=${1:?label required}
bin=${2:-"$root/dist/skillctl"}
home=/private/tmp/skillctl-dev-home
remote=https://github.com/mattpocock/skills
out="$root/.scratch/git-source-speed/measurements/${label}"

mkdir -p "$out"
rm -rf "$home/.skillctl"
mkdir -p "$home"

ts() { python3 -c 'import time; print(f"{time.time():.3f}")'; }

echo "==> init ($label)"
HOME="$home" "$bin" init >/dev/null

echo "==> source add (cold cache)"
t0=$(ts)
HOME="$home" SKILLCTL_GIT_TRACE="${SKILLCTL_GIT_TRACE:-}" "$bin" source add "$remote" --name skills >"$out/add.stdout" 2>"$out/add.stderr" || {
  echo "source add failed" >&2
  cat "$out/add.stderr" >&2
  exit 1
}
t1=$(ts)
python3 -c "print(f'{float('$t1')-float('$t0'):.3f}')" >"$out/add.seconds"

echo "==> skill import --all"
t0=$(ts)
HOME="$home" SKILLCTL_GIT_TRACE="${SKILLCTL_GIT_TRACE:-}" "$bin" skill import --source skills --all >"$out/import.stdout" 2>"$out/import.stderr" || {
  echo "skill import failed" >&2
  cat "$out/import.stderr" >&2
  exit 1
}
t1=$(ts)
python3 -c "print(f'{float('$t1')-float('$t0'):.3f}')" >"$out/import.seconds"

echo "==> source check (warm cache, possibly same commit)"
t0=$(ts)
HOME="$home" SKILLCTL_GIT_TRACE="${SKILLCTL_GIT_TRACE:-}" "$bin" source check skills >"$out/check.stdout" 2>"$out/check.stderr" || {
  echo "source check failed" >&2
  cat "$out/check.stderr" >&2
  exit 1
}
t1=$(ts)
python3 -c "print(f'{float('$t1')-float('$t0'):.3f}')" >"$out/check.seconds"

{
  echo "# $label"
  echo
  echo "- home: \`$home\`"
  echo "- remote: \`$remote\`"
  echo "- binary: \`$bin\`"
  echo "- git-cache deleted before init: yes"
  echo
  echo "| step | seconds |"
  echo "| --- | ---: |"
  echo "| source add | $(cat "$out/add.seconds") |"
  echo "| skill import --all | $(cat "$out/import.seconds") |"
  echo "| source check | $(cat "$out/check.seconds") |"
  echo
  echo "## add stdout"
  echo
  echo '```'
  cat "$out/add.stdout"
  echo '```'
  echo
  echo "## import stdout (last 20 lines)"
  echo
  echo '```'
  tail -n 20 "$out/import.stdout"
  echo '```'
} >"$out/README.md"

echo "wrote $out/README.md"
cat "$out/README.md"
