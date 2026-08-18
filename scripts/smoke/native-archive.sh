#!/usr/bin/env bash
# Build the current host's CGO-free archive and run the native smoke.
set -euo pipefail

root=$(cd -P "$(dirname "$0")/../.." && pwd)
cd "$root"

case $(uname -s) in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *)
    echo "unsupported host OS $(uname -s)" >&2
    exit 1
    ;;
esac
case $(uname -m) in
  x86_64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *)
    echo "unsupported host arch $(uname -m)" >&2
    exit 1
    ;;
esac

webui="$root/internal/webui"
if [ ! -f "$webui/dist/index.html" ]; then
  echo "==> frontend"
  (cd "$webui" && npm ci && npx vite build)
fi

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
export COPYFILE_DISABLE=1
echo "==> build ${os}/${arch}"
CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -o "$stage/skillctl" ./cmd/skillctl
name="skillctl_ci_${os}_${arch}.tar.gz"
tar -C "$stage" -czf "$stage/$name" skillctl
echo "==> smoke ${os}/${arch}"
"$root/scripts/smoke/archive.sh" "$stage/$name"
