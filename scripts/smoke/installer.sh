#!/usr/bin/env bash
# Exercise install.sh against local release fixtures; no network is used.
set -euo pipefail

root=$(cd -P "$(dirname "$0")/../.." && pwd)
installer=$root/install.sh

case $(uname -s) in
  Darwin) target_os=darwin ;;
  Linux) target_os=linux ;;
  *)
    echo "installer smoke requires macOS or Linux" >&2
    exit 1
    ;;
esac
case $(uname -m) in
  x86_64 | amd64) target_arch=amd64 ;;
  arm64 | aarch64) target_arch=arm64 ;;
  *)
    echo "installer smoke requires amd64 or arm64" >&2
    exit 1
    ;;
esac

temp=$(mktemp -d "${TMPDIR:-/tmp}/skillctl-installer-smoke.XXXXXX")
trap 'rm -rf "$temp"' EXIT
release_root=$temp/releases

write_checksum() {
  directory=$1
  archive_name=$2
  (
    cd "$directory"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$archive_name" >SHA256SUMS
    else
      shasum -a 256 "$archive_name" >SHA256SUMS
    fi
  )
}

make_release() {
  version=$1
  release_directory=$2
  stage=$temp/stage-$version
  archive_name=skillctl_${version}_${target_os}_${target_arch}.tar.gz

  mkdir -p "$stage" "$release_directory"
  cat >"$stage/skillctl" <<EOF
#!/bin/sh
printf '%s\\n' 'skillctl fixture ${version}'
EOF
  chmod 0755 "$stage/skillctl"
  tar -C "$stage" -czf "$release_directory/$archive_name" skillctl
  write_checksum "$release_directory" "$archive_name"
}

expect_failure() {
  description=$1
  shift
  if "$@" >"$temp/unexpected.stdout" 2>"$temp/unexpected.stderr"; then
    echo "expected failure: $description" >&2
    cat "$temp/unexpected.stdout" >&2
    cat "$temp/unexpected.stderr" >&2
    exit 1
  fi
}

latest_version=v9.8.7
latest_directory=$release_root/latest/download
make_release "$latest_version" "$latest_directory"

latest_install="$temp/install latest with spaces"
mkdir -p "$latest_install"
echo "==> install latest into a path containing spaces"
PATH="$latest_install:$PATH" \
  SKILLCTL_RELEASE_ROOT="file://$release_root" \
  "$installer" --install-dir "$latest_install"
latest_output=$("$latest_install/skillctl")
[ "$latest_output" = "skillctl fixture $latest_version" ] || {
  echo "unexpected latest binary output: $latest_output" >&2
  exit 1
}

pinned_version=v8.7.6
pinned_directory=$release_root/download/$pinned_version
make_release "$pinned_version" "$pinned_directory"
pinned_install=$temp/pinned-bin

echo "==> install a pinned version"
PATH="$pinned_install:$PATH" \
  SKILLCTL_RELEASE_ROOT="file://$release_root" \
  "$installer" --version "$pinned_version" --install-dir "$pinned_install"
pinned_output=$("$pinned_install/skillctl")
[ "$pinned_output" = "skillctl fixture $pinned_version" ] || {
  echo "unexpected pinned binary output: $pinned_output" >&2
  exit 1
}

corrupt_version=v7.6.5
corrupt_directory=$release_root/download/$corrupt_version
make_release "$corrupt_version" "$corrupt_directory"
printf 'corruption\n' >>"$corrupt_directory/skillctl_${corrupt_version}_${target_os}_${target_arch}.tar.gz"
corrupt_install=$temp/corrupt-bin
mkdir -p "$corrupt_install"
printf '#!/bin/sh\nprintf "existing binary\\n"\n' >"$corrupt_install/skillctl"
chmod 0755 "$corrupt_install/skillctl"

echo "==> reject a corrupt archive without replacing the current binary"
expect_failure "corrupt archive" env \
  SKILLCTL_RELEASE_ROOT="file://$release_root" \
  "$installer" --version "$corrupt_version" --install-dir "$corrupt_install"
existing_output=$("$corrupt_install/skillctl")
[ "$existing_output" = "existing binary" ] || {
  echo "corrupt release replaced the existing installation" >&2
  exit 1
}

echo "==> reject invalid input before downloading"
expect_failure "invalid version" "$installer" --version 1.2.3 --install-dir "$temp/invalid"
expect_failure "unknown option" "$installer" --unknown

if find "$latest_install" "$pinned_install" "$corrupt_install" -name '.skillctl.install.*' -print | grep -q .; then
  echo "installer left a pending installation file behind" >&2
  exit 1
fi

echo "installer smoke passed"
