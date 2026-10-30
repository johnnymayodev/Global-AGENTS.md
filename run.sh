#!/bin/sh
set -eu

# Standalone runner for agent-links.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
#
# Downloads the released standalone executable for this machine, verifies its
# SHA-256 checksum, and caches it under
# ${XDG_CACHE_HOME:-$HOME/.cache}/agent-links/v<version>/<target>/ so later
# runs need no network. The cached asset is re-verified on every run. The CLI
# runs with the caller's arguments, working directory, environment, and umask.
# No runtime, PATH change, or installation is required.

repository=johnnymayodev/Global-AGENTS.md
version=0.5.2

os=$(uname -s)
machine=$(uname -m)

case "$os:$machine" in
  Darwin:x86_64 | Darwin:amd64) target=darwin-x64 ;;
  Darwin:arm64 | Darwin:aarch64) target=darwin-arm64 ;;
  Linux:x86_64 | Linux:amd64) target=linux-x64 ;;
  Linux:arm64 | Linux:aarch64) target=linux-arm64 ;;
  *)
    printf 'error: unsupported platform: %s %s\n' "$os" "$machine" >&2
    printf 'supported: macOS and Linux (including musl-based distributions and WSL), x64 and arm64\n' >&2
    exit 2
    ;;
esac

# The checksum utility is required for both the cache-hit and download paths.
if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{ print $1 }'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{ print $1 }'; }
else
  printf 'error: sha256sum or shasum is required to verify the download\n' >&2
  exit 127
fi

asset="agent-links-$target"
base_url="https://github.com/${repository}/releases/download/v${version}"

work_dir=""
temp_asset=""
temp_sums=""
temp_in_cache=0

cleanup() {
  if [ -n "$temp_asset" ]; then
    rm -f "$temp_asset"
  fi
  if [ -n "$temp_sums" ]; then
    rm -f "$temp_sums"
  fi
  if [ -n "$work_dir" ]; then
    rm -rf "$work_dir"
  fi
  return 0
}
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

# cache_dir_ok reports whether dir exists, belongs to the current user, and is
# not group- or world-writable. The trailing slash follows a symlinked cache
# root (a common ~/.cache setup) and inspects the directory it points at.
# Ownership uses find -user rather than test -O, which dash's test rejects.
cache_dir_ok() {
  if [ ! -d "$1" ]; then
    return 1
  fi
  cache_bad=$(find "$1/" -maxdepth 0 \
    \( ! -user "$(id -u)" -o -perm -020 -o -perm -002 \) 2>/dev/null) || return 1
  [ -z "$cache_bad" ]
}

# cache_base is the XDG cache root when it is absolute, else $HOME/.cache.
cache_base=""
if [ -n "${XDG_CACHE_HOME:-}" ]; then
  case "$XDG_CACHE_HOME" in
    /*) cache_base=$XDG_CACHE_HOME ;;
  esac
fi
if [ -z "$cache_base" ] && [ -n "${HOME:-}" ]; then
  case "$HOME" in
    /*) cache_base=$HOME/.cache ;;
  esac
fi

# prepare_cache validates or creates the version-scoped leaf. Any unusable
# path falls back to a private temporary directory instead of failing the run.
prepare_cache() {
  [ -n "$cache_base" ] || return 1

  # Missing directories are created with a private umask. The umask is scoped
  # to these subshells so the CLI still inherits the caller's umask.
  if [ ! -d "$cache_base" ]; then
    (umask 077; mkdir -p "$cache_base") 2>/dev/null || return 1
  fi
  cache_dir_ok "$cache_base" || return 1

  leaf="$cache_base/agent-links/v$version/$target"
  # Reject symlinked managed components before creating anything below them.
  [ ! -L "$cache_base/agent-links" ] || return 1
  [ ! -L "$cache_base/agent-links/v$version" ] || return 1
  if [ ! -d "$leaf" ]; then
    (umask 077; mkdir -p "$leaf") 2>/dev/null || return 1
  fi

  # Keep the leaf private and owned by this user. A corrupt leaf is replaced
  # by the mktemp fallback.
  [ ! -L "$cache_base/agent-links" ] || return 1
  [ ! -L "$cache_base/agent-links/v$version" ] || return 1
  [ ! -L "$leaf" ] || return 1
  cache_dir_ok "$cache_base/agent-links" || return 1
  cache_dir_ok "$cache_base/agent-links/v$version" || return 1
  chmod 700 "$leaf" 2>/dev/null || return 1
  cache_dir_ok "$leaf" || return 1

  cache_leaf=$leaf
  cache_asset="$leaf/$asset"
  cache_sums="$leaf/checksums-$target.txt"
  return 0
}

# owned_by_user reports whether path is owned by the current user. This is the
# portable form of test -O, which not every /bin/sh supports.
owned_by_user() {
  owner=$(find "$1" -maxdepth 0 -user "$(id -u)" 2>/dev/null) || return 1
  [ -n "$owner" ]
}

# cache_checksum prints the entry for the asset, if any.
cache_checksum() {
  awk -v name="$asset" '$2 == name { print $1 }' "$1"
}

# cache_entry_valid re-verifies a warm cache: both entries must be private
# regular files, and the asset must match exactly one 64-hex SHA-256 entry.
cache_entry_valid() {
  [ -n "${cache_leaf:-}" ] || return 1
  if [ ! -f "$cache_asset" ] || [ -L "$cache_asset" ] || [ ! -s "$cache_asset" ]; then
    return 1
  fi
  if [ ! -f "$cache_sums" ] || [ -L "$cache_sums" ] || [ ! -s "$cache_sums" ]; then
    return 1
  fi
  if ! owned_by_user "$cache_asset" || ! owned_by_user "$cache_sums"; then
    return 1
  fi

  expected=$(cache_checksum "$cache_sums") || return 1
  case "$expected" in
    "" | *[!0-9a-fA-F]*) return 1 ;;
  esac
  [ "${#expected}" -eq 64 ] || return 1

  actual=$(sha256 "$cache_asset") || return 1
  [ "$actual" = "$expected" ] || return 1

  chmod 700 "$cache_asset" 2>/dev/null || return 1
  chmod 600 "$cache_sums" 2>/dev/null || return 1
  return 0
}

download() {
  if ! curl --fail --location --silent --show-error \
    --retry 2 --retry-delay 1 --connect-timeout 10 \
    --proto '=https' --proto-redir '=https' \
    --output "$1" "$base_url/$2"; then
    printf 'error: failed to download %s from %s\n' "$2" "$base_url" >&2
    printf 'check your network connection and that release v%s exists for %s\n' \
      "$version" "$repository" >&2
    exit 1
  fi
}

# claim_temp_files uses the cache directory when it is usable, so the final
# rename stays on one filesystem and is atomic; otherwise it uses a fresh
# mktemp directory.
claim_temp_files() {
  if [ -n "${cache_leaf:-}" ]; then
    if temp_asset=$(mktemp "$cache_leaf/.agent-links.XXXXXX" 2>/dev/null); then
      if temp_sums=$(mktemp "$cache_leaf/.checksums.XXXXXX" 2>/dev/null); then
        temp_in_cache=1
      else
        rm -f "$temp_asset"
        temp_asset=""
      fi
    else
      temp_asset=""
    fi
  fi

  if [ -z "$temp_asset" ] || [ -z "$temp_sums" ]; then
    if ! work_dir=$(mktemp -d "${TMPDIR:-/tmp}/agent-links.XXXXXX" 2>/dev/null); then
      printf 'error: could not create a temporary directory in %s\n' "${TMPDIR:-/tmp}" >&2
      exit 1
    fi
    temp_asset="$work_dir/$asset"
    temp_sums="$work_dir/checksums-$target.txt"
    temp_in_cache=0
  fi
}

cache_leaf=""
cache_asset=""
cache_sums=""
prepare_cache || true

if [ -n "$cache_leaf" ] && cache_entry_valid; then
  asset_path=$cache_asset
else
  if ! command -v curl >/dev/null 2>&1; then
    printf 'error: curl is required to download agent-links\n' >&2
    exit 127
  fi

  if ! command -v mktemp >/dev/null 2>&1; then
    printf 'error: mktemp is required to create a temporary directory\n' >&2
    exit 127
  fi

  claim_temp_files
  download "$temp_asset" "$asset"
  download "$temp_sums" "checksums-$target.txt"

  expected=$(cache_checksum "$temp_sums")
  if [ -z "$expected" ]; then
    printf 'error: no checksum entry for %s in release v%s\n' "$asset" "$version" >&2
    exit 1
  fi
  case "$expected" in
    *[!0-9a-fA-F]*)
      printf 'error: no checksum entry for %s in release v%s\n' "$asset" "$version" >&2
      exit 1
      ;;
  esac
  if [ "${#expected}" -ne 64 ]; then
    printf 'error: no checksum entry for %s in release v%s\n' "$asset" "$version" >&2
    exit 1
  fi

  actual=$(sha256 "$temp_asset")
  if [ "$actual" != "$expected" ]; then
    printf 'error: checksum verification failed for %s\n' "$asset" >&2
    printf 'expected: %s\n' "$expected" >&2
    printf 'actual:   %s\n' "$actual" >&2
    printf 'refusing to run a corrupted or tampered download\n' >&2
    exit 1
  fi

  if ! chmod 700 "$temp_asset"; then
    printf 'error: could not make the downloaded %s executable\n' "$asset" >&2
    exit 1
  fi
  if ! chmod 600 "$temp_sums" 2>/dev/null; then
    printf 'error: could not secure the downloaded checksum file\n' >&2
    exit 1
  fi

  asset_path=$temp_asset
  if [ -n "$cache_leaf" ] && [ "$temp_in_cache" -eq 1 ]; then
    # The replacement is verified before either rename. If the pair is ever
    # left inconsistent, the hit check rejects it and downloads again.
    if mv -f "$temp_sums" "$cache_sums" 2>/dev/null && mv -f "$temp_asset" "$cache_asset" 2>/dev/null; then
      temp_sums=""
      temp_asset=""
      asset_path=$cache_asset
    else
      printf 'warning: could not update the agent-links cache; running from a temporary directory\n' >&2
    fi
  elif [ -n "$cache_leaf" ]; then
    printf 'warning: could not prepare the agent-links cache; running from a temporary directory\n' >&2
  fi
fi

status=0
if [ -t 1 ] && ( : </dev/tty ) 2>/dev/null; then
  # Reconnect stdin from the controlling terminal so the interactive picker,
  # plan review, and confirmation work when this script itself was piped into
  # a shell (curl ... | bash).
  "$asset_path" "$@" </dev/tty || status=$?
else
  "$asset_path" "$@" || status=$?
fi
if [ "$status" -eq 126 ] || [ "$status" -eq 127 ]; then
  printf 'error: could not launch %s; check that the release asset matches this operating system and architecture\n' "$asset" >&2
fi
exit "$status"
