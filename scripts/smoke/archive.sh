#!/usr/bin/env bash
# Native archive smoke: extract skillctl into an isolated HOME and empty
# read-only cwd with no Node or repository files, then prove init, embedded
# migrations, ui, API, hashed assets, a SPA deep link, and a clean shutdown.
# On Linux, also prove the binary has no CGO/system-SQLite dynamic dependency.
set -euo pipefail

usage() {
  echo "usage: $0 <archive.tar.gz> [version] [commit]" >&2
  exit 2
}

[ $# -ge 1 ] && [ $# -le 3 ] || usage

archive=$1
expect_version=${2:-}
expect_commit=${3:-}

if [ ! -f "$archive" ]; then
  echo "archive not found: ${archive}" >&2
  exit 1
fi

scratch=$(mktemp -d)
ui_pid=
cleanup() {
  if [ -n "${ui_pid:-}" ]; then
    kill -TERM "$ui_pid" 2>/dev/null || true
    wait "$ui_pid" 2>/dev/null || true
  fi
  chmod -R u+w "$scratch" 2>/dev/null || true
  rm -rf "$scratch"
}
trap cleanup EXIT

extract=$scratch/extract
home=$scratch/home
cwd=$scratch/cwd
tmpdir=$scratch/tmp
mkdir -p "$extract" "$home" "$cwd" "$tmpdir"

tar -C "$extract" -xzf "$archive"
entries=$(tar -tzf "$archive" | sed 's#^\./##' | grep -v '/$' || true)
if [ "$entries" != "skillctl" ]; then
  echo "archive must contain only skillctl, got:" >&2
  echo "$entries" >&2
  exit 1
fi
bin=$extract/skillctl
if [ ! -x "$bin" ]; then
  echo "extracted skillctl is not executable" >&2
  exit 1
fi

if [ -e "$cwd/package.json" ] || [ -e "$cwd/node_modules" ] || [ -e "$cwd/go.mod" ]; then
  echo "cwd is not empty of repository files" >&2
  exit 1
fi

# skillctl runs with a controlled empty PATH so it cannot see Node or any
# other host tool. Launch through absolute /usr/bin/env. The smoke script's
# own curl/tar keep the caller PATH.
bin_env=(
  /usr/bin/env -i
  HOME="$home"
  TMPDIR="$tmpdir"
  PATH=
)
run_bin() {
  "${bin_env[@]}" "$bin" "$@"
}
if /usr/bin/env -i HOME="$home" TMPDIR="$tmpdir" PATH= node -v >/dev/null 2>&1; then
  echo "node is reachable with the controlled empty PATH" >&2
  exit 1
fi

if [ "$(uname -s)" = Linux ]; then
  if command -v ldd >/dev/null 2>&1; then
    if ldd "$bin" >/dev/null 2>&1; then
      echo "Linux archive is dynamically linked:" >&2
      ldd "$bin" >&2
      if ldd "$bin" | grep -qi sqlite; then
        echo "binary links a system SQLite" >&2
      fi
      exit 1
    fi
  fi
fi

chmod a-w "$cwd"
cd "$cwd"

echo "==> --version"
ver_out=$(run_bin --version)
echo "$ver_out"
if [ -n "$expect_version" ] && ! printf '%s\n' "$ver_out" | grep -Fq "$expect_version"; then
  echo "--version did not contain ${expect_version}: ${ver_out}" >&2
  exit 1
fi
if [ -n "$expect_commit" ] && ! printf '%s\n' "$ver_out" | grep -Fq "$expect_commit"; then
  echo "--version did not contain ${expect_commit}: ${ver_out}" >&2
  exit 1
fi

echo "==> init"
run_bin init
if [ ! -f "$home/.skillctl/config.toml" ] || [ ! -f "$home/.skillctl/state.db" ]; then
  echo "init did not write config.toml and state.db under HOME" >&2
  exit 1
fi

status_out=$(run_bin status)
echo "$status_out"
if ! printf '%s\n' "$status_out" | grep -Fq "Status: ready"; then
  echo "status after init is not ready: ${status_out}" >&2
  exit 1
fi

# Query a table created by the embedded migrations. An empty collection
# proves the schema exists; a missing table would fail the command.
echo "==> source list --json"
list_out=$(run_bin source list --json)
echo "$list_out"
if ! printf '%s\n' "$list_out" | grep -Fq '"items": []'; then
  echo "source list --json was not an empty items array: ${list_out}" >&2
  exit 1
fi
if ! printf '%s\n' "$list_out" | grep -Fq '"total": 0'; then
  echo "source list --json was not an empty collection: ${list_out}" >&2
  exit 1
fi

echo "==> ui"
ui_out=$scratch/ui.out
ui_err=$scratch/ui.err
# exec so $! is skillctl, not a bash wrapper that exits 143 on SIGTERM.
"${bin_env[@]}" "$bin" ui --port 0 --no-open >"$ui_out" 2>"$ui_err" &
ui_pid=$!

url=
deadline=$((SECONDS + 15))
while [ $SECONDS -lt $deadline ]; do
  if ! kill -0 "$ui_pid" 2>/dev/null; then
    wait "$ui_pid" || true
    echo "ui exited before printing its URL" >&2
    echo "stderr:" >&2
    cat "$ui_err" >&2
    echo "stdout:" >&2
    cat "$ui_out" >&2
    exit 1
  fi
  url=$(sed -n 's/^UI available at //p' "$ui_out" | head -n 1)
  if [ -n "$url" ]; then
    break
  fi
  sleep 0.1
done
if [ -z "$url" ]; then
  echo "ui did not print its URL within 15s" >&2
  cat "$ui_err" >&2
  exit 1
fi
echo "ui at ${url}"

api=$(curl -fsS "$url/api/v1/status")
echo "api: ${api}"
if ! printf '%s\n' "$api" | grep -Fq '"state":"ready"'; then
  echo "GET /api/v1/status was not ready: ${api}" >&2
  exit 1
fi

html=$(curl -fsS "$url/")
if ! printf '%s\n' "$html" | grep -Fq '<div id="root">'; then
  echo "GET / did not return the SPA shell" >&2
  exit 1
fi

asset=$(printf '%s\n' "$html" | sed -n 's/.*src="\([^"]*\)".*/\1/p' | head -n 1)
case $asset in
  /assets/*) ;;
  *)
    echo "index.html has no hashed /assets script, got ${asset:-<empty>}" >&2
    exit 1
    ;;
esac
asset_body=$(curl -fsS "$url$asset")
if [ -z "$asset_body" ]; then
  echo "GET ${asset} was empty" >&2
  exit 1
fi
echo "hashed asset: ${asset} (${#asset_body} bytes)"

deep=$(curl -fsS "$url/skills/example-skill?tab=synchronization")
if ! printf '%s\n' "$deep" | grep -Fq '<div id="root">'; then
  echo "SPA deep link did not return the shell" >&2
  exit 1
fi

echo "==> shutdown"
kill -TERM "$ui_pid"
down_deadline=$((SECONDS + 10))
while kill -0 "$ui_pid" 2>/dev/null; do
  if [ $SECONDS -ge $down_deadline ]; then
    kill -KILL "$ui_pid" 2>/dev/null || true
    echo "ui did not shut down after SIGTERM" >&2
    exit 1
  fi
  sleep 0.1
done
ui_status=0
wait "$ui_pid" || ui_status=$?
ui_pid=
if [ "$ui_status" -ne 0 ]; then
  echo "ui exited ${ui_status} after SIGTERM" >&2
  cat "$ui_err" >&2
  exit 1
fi

# cwd was empty and read-only; the binary must not have written here.
leftovers=$(find "$cwd" -mindepth 1 -print || true)
if [ -n "$leftovers" ]; then
  echo "cwd is no longer empty:" >&2
  echo "$leftovers" >&2
  exit 1
fi

echo "archive smoke passed"
