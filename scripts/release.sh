#!/usr/bin/env bash
# Validate a clean exact-tag checkout, run checks, and write the four
# CGO-free archives plus SHA256SUMS. Does not create a tag or upload.
# Only the current platform's archive is smoked; the others are built
# and reported as needing a native runner.
set -euo pipefail

usage() {
  echo "usage: $0 vMAJOR.MINOR.PATCH" >&2
  exit 2
}

[ $# -eq 1 ] || usage
version=$1
if ! [[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must be vMAJOR.MINOR.PATCH, got ${version}" >&2
  exit 2
fi

# Physical path: macOS /var is a symlink to /private/var; Go filepath
# resolves the real path, so a logical cwd breaks NormalizeLocal.
root=$(cd -P "$(dirname "$0")/.." && pwd)
cd "$root"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "release requires a git checkout" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "worktree is not clean; commit or stash changes before releasing" >&2
  git status --porcelain >&2
  exit 1
fi

if ! git rev-parse --verify --quiet "refs/tags/${version}" >/dev/null; then
  echo "tag ${version} does not exist; create it before releasing" >&2
  exit 1
fi

head=$(git rev-parse HEAD)
tagged=$(git rev-parse "${version}^{commit}")
if [ "$head" != "$tagged" ]; then
  echo "HEAD (${head}) is not exactly tag ${version} (${tagged})" >&2
  exit 1
fi

commit=$(git rev-parse HEAD)
ldflags="-X skillctl/internal/cli.version=${version} -X skillctl/internal/cli.gitCommit=${commit}"

case $(uname -s) in
  Darwin) native_os=darwin ;;
  Linux) native_os=linux ;;
  *)
    echo "unsupported host OS $(uname -s)" >&2
    exit 1
    ;;
esac
case $(uname -m) in
  x86_64) native_arch=amd64 ;;
  arm64 | aarch64) native_arch=arm64 ;;
  *)
    echo "unsupported host arch $(uname -m)" >&2
    exit 1
    ;;
esac
native="${native_os}/${native_arch}"

echo "==> checks"
"$root/scripts/check.sh"

echo "==> README quickstart"
mkdir -p "$root/dist"
CGO_ENABLED=0 go build -ldflags "$ldflags" -o "$root/dist/skillctl" ./cmd/skillctl
"$root/scripts/smoke/readme.sh"

echo "==> dist"
rm -rf "$root/dist"
mkdir -p "$root/dist"

if [ ! -f "$root/internal/webui/dist/index.html" ]; then
  echo "checks did not leave a Vite dist; refusing to compile" >&2
  exit 1
fi

# macOS tar otherwise injects AppleDouble files into the archive.
export COPYFILE_DISABLE=1

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

smoked=
for spec in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
  os=${spec%/*}
  arch=${spec#*/}
  name="skillctl_${version}_${os}_${arch}.tar.gz"
  echo "==> build ${os}/${arch}"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags "$ldflags" -o "$stage/skillctl" ./cmd/skillctl
  tar -C "$stage" -czf "$root/dist/$name" skillctl

  if [ "$spec" = "$native" ]; then
    echo "==> smoke ${spec}"
    "$root/scripts/smoke/archive.sh" "$root/dist/$name" "$version" "$commit"
    smoked=$spec
  else
    echo "note: ${spec} archive built; needs a native runner to smoke"
  fi
done

if [ -z "$smoked" ]; then
  echo "this host (${native}) is not a supported release platform; archives were built but not smoked" >&2
  exit 1
fi

echo "==> SHA256SUMS"
(
  cd "$root/dist"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum skillctl_*.tar.gz >SHA256SUMS
  else
    shasum -a 256 skillctl_*.tar.gz >SHA256SUMS
  fi
)
sum_lines=$(grep -c . "$root/dist/SHA256SUMS" || true)
if [ "$sum_lines" -ne 4 ]; then
  echo "SHA256SUMS must list exactly four archives, got ${sum_lines}" >&2
  exit 1
fi

want=$(printf '%s\n' \
  SHA256SUMS \
  "skillctl_${version}_darwin_amd64.tar.gz" \
  "skillctl_${version}_darwin_arm64.tar.gz" \
  "skillctl_${version}_linux_amd64.tar.gz" \
  "skillctl_${version}_linux_arm64.tar.gz" | sort)
got=$(cd "$root/dist" && ls -1 | sort)
if [ "$got" != "$want" ]; then
  echo "dist must contain only the four archives and SHA256SUMS, got:" >&2
  echo "$got" >&2
  exit 1
fi

echo "release ${version} (${commit})"
echo "smoked: ${smoked}"
echo "needs native runner: the other three archives"
ls -l "$root/dist"
