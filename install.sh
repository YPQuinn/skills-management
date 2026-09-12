#!/bin/sh
# Install a checksummed skillctl release without requiring elevated privileges.
set -eu

REPOSITORY=YPQuinn/skills-management
DEFAULT_RELEASE_ROOT="https://github.com/${REPOSITORY}/releases"

usage() {
  cat <<'EOF'
Install skillctl from GitHub Releases.

Usage:
  install.sh [--version vMAJOR.MINOR.PATCH] [--install-dir DIRECTORY]
  install.sh --help

Options:
  --version VERSION       Install a specific stable version (default: latest).
  --install-dir DIRECTORY Install into DIRECTORY (default: ~/.local/bin).

The SKILLCTL_VERSION and SKILLCTL_INSTALL_DIR environment variables provide
matching defaults. Command-line options take precedence.
EOF
}

fail() {
  printf 'skillctl installer: %s\n' "$*" >&2
  exit 1
}

version=${SKILLCTL_VERSION:-latest}
install_dir=${SKILLCTL_INSTALL_DIR:-}

while [ "$#" -gt 0 ]; do
  case $1 in
    --version)
      [ "$#" -ge 2 ] || fail "--version requires a value"
      version=$2
      shift 2
      ;;
    --version=*)
      version=${1#*=}
      shift
      ;;
    --install-dir)
      [ "$#" -ge 2 ] || fail "--install-dir requires a value"
      install_dir=$2
      shift 2
      ;;
    --install-dir=*)
      install_dir=${1#*=}
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1"
      ;;
  esac
done

if [ "$version" != latest ] && ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  fail "version must be 'latest' or vMAJOR.MINOR.PATCH, got: $version"
fi

if [ -z "$install_dir" ]; then
  [ -n "${HOME:-}" ] || fail "HOME is not set; pass --install-dir DIRECTORY"
  install_dir=$HOME/.local/bin
fi

case $(uname -s) in
  Darwin) target_os=darwin ;;
  Linux) target_os=linux ;;
  *) fail "unsupported operating system: $(uname -s) (supported: macOS and Linux)" ;;
esac

case $(uname -m) in
  x86_64 | amd64) target_arch=amd64 ;;
  arm64 | aarch64) target_arch=arm64 ;;
  *) fail "unsupported architecture: $(uname -m) (supported: amd64 and arm64)" ;;
esac

download() {
  source_url=$1
  destination=$2

  if command -v curl >/dev/null 2>&1; then
    case $source_url in
      https://*) curl --proto '=https' --proto-redir '=https' -fsSL "$source_url" -o "$destination" ;;
      *) curl -fsSL "$source_url" -o "$destination" ;;
    esac
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$source_url" -O "$destination"
  else
    fail "curl or wget is required to download release files"
  fi
}

verify_checksum() {
  checksum_file=$1
  archive_path=$2
  archive_name=$3
  selected_checksum=$4

  checksum_count=$(awk -v name="$archive_name" '$2 == name { count++ } END { print count + 0 }' "$checksum_file")
  [ "$checksum_count" -eq 1 ] || fail "SHA256SUMS does not contain exactly one entry for $archive_name"
  awk -v name="$archive_name" '$2 == name { print $1 "  " $2 }' "$checksum_file" >"$selected_checksum"

  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$(dirname "$archive_path")" && sha256sum -c "$(basename "$selected_checksum")")
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$(dirname "$archive_path")" && shasum -a 256 -c "$(basename "$selected_checksum")")
  else
    fail "sha256sum or shasum is required to verify the release"
  fi
}

release_root=${SKILLCTL_RELEASE_ROOT:-$DEFAULT_RELEASE_ROOT}
release_root=${release_root%/}
if [ "$version" = latest ]; then
  release_url=$release_root/latest/download
else
  release_url=$release_root/download/$version
fi

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/skillctl-install.XXXXXX") || fail "could not create a temporary directory"
pending_install=
cleanup() {
  rm -rf "$temp_dir"
  if [ -n "$pending_install" ]; then
    rm -f "$pending_install"
  fi
}
trap cleanup EXIT HUP INT TERM

checksums=$temp_dir/SHA256SUMS
printf 'Downloading checksums...\n'
download "$release_url/SHA256SUMS" "$checksums" || fail "could not download $release_url/SHA256SUMS"

if [ "$version" = latest ]; then
  archive_name=$(awk -v suffix="_${target_os}_${target_arch}.tar.gz" '
    $2 ~ /^skillctl_v[0-9]+\.[0-9]+\.[0-9]+_/ &&
    length($2) > length(suffix) &&
    substr($2, length($2) - length(suffix) + 1) == suffix { print $2 }
  ' "$checksums")
  case $archive_name in
    *"
"* | "") fail "could not select one ${target_os}/${target_arch} archive from SHA256SUMS" ;;
  esac
  version=${archive_name#skillctl_}
  version=${version%_${target_os}_${target_arch}.tar.gz}
else
  archive_name=skillctl_${version}_${target_os}_${target_arch}.tar.gz
fi

case $archive_name in
  skillctl_v[0-9]*.[0-9]*.[0-9]*_${target_os}_${target_arch}.tar.gz) ;;
  *) fail "unsafe archive name selected from SHA256SUMS: $archive_name" ;;
esac

archive_path=$temp_dir/$archive_name
printf 'Downloading skillctl %s for %s/%s...\n' "$version" "$target_os" "$target_arch"
download "$release_url/$archive_name" "$archive_path" || fail "could not download $release_url/$archive_name"

printf 'Verifying SHA-256 checksum...\n'
verify_checksum "$checksums" "$archive_path" "$archive_name" "$temp_dir/SELECTED_SHA256SUM"

extract_dir=$temp_dir/extract
mkdir "$extract_dir"
tar -xzf "$archive_path" -C "$extract_dir" skillctl || fail "could not extract skillctl from $archive_name"
[ -f "$extract_dir/skillctl" ] && [ ! -L "$extract_dir/skillctl" ] || fail "release archive does not contain a regular skillctl file"

mkdir -p "$install_dir" || fail "could not create installation directory: $install_dir"
[ ! -d "$install_dir/skillctl" ] || fail "installation target is a directory: $install_dir/skillctl"
pending_install=$install_dir/.skillctl.install.$$
cp "$extract_dir/skillctl" "$pending_install" || fail "could not write to installation directory: $install_dir"
chmod 0755 "$pending_install" || fail "could not make the installed binary executable"
mv -f "$pending_install" "$install_dir/skillctl" || fail "could not install skillctl into $install_dir"
pending_install=

printf 'Installed skillctl %s to %s/skillctl\n' "$version" "$install_dir"
case :${PATH:-}: in
  *:"$install_dir":*) ;;
  *)
    printf 'Add %s to PATH before running skillctl.\n' "$install_dir" >&2
    ;;
esac
