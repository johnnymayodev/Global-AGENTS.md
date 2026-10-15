#!/bin/sh
set -eu

# Standalone runner for agent-links.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
#
# Downloads the released standalone executable for this machine into a private
# temporary directory, verifies its SHA-256 checksum, runs it with the caller's
# arguments and working directory, then removes the temporary files. No runtime
# (Bun or Node.js), PATH change, or installation is required.

repository=johnnymayodev/Global-AGENTS.md
version=0.1.0

os=$(uname -s)
machine=$(uname -m)

case "$os:$machine" in
  Darwin:x86_64 | Darwin:amd64) target=darwin-x64 ;;
  Darwin:arm64 | Darwin:aarch64) target=darwin-arm64 ;;
  Linux:x86_64 | Linux:amd64) target=linux-x64 ;;
  Linux:arm64 | Linux:aarch64) target=linux-arm64 ;;
  *)
    printf 'error: unsupported platform: %s %s\n' "$os" "$machine" >&2
    printf 'supported: macOS and glibc-based Linux (including WSL), x64 and arm64\n' >&2
    exit 2
    ;;
esac

if [ "$target" = linux-x64 ] || [ "$target" = linux-arm64 ]; then
  if ls /lib/ld-musl-* >/dev/null 2>&1; then
    printf 'error: musl-based Linux (for example Alpine) is not supported; use a glibc-based distribution or WSL\n' >&2
    exit 2
  fi
fi

if ! command -v curl >/dev/null 2>&1; then
  printf 'error: curl is required to download agent-links\n' >&2
  exit 127
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | awk '{ print $1 }'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{ print $1 }'; }
else
  printf 'error: sha256sum or shasum is required to verify the download\n' >&2
  exit 127
fi

if ! command -v mktemp >/dev/null 2>&1; then
  printf 'error: mktemp is required to create a temporary directory\n' >&2
  exit 127
fi

if ! temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/agent-links.XXXXXX" 2>/dev/null); then
  printf 'error: could not create a temporary directory in %s\n' "${TMPDIR:-/tmp}" >&2
  exit 1
fi

cleanup() {
  rm -rf "$temp_dir"
}
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

asset="agent-links-$target"
base_url="https://github.com/${repository}/releases/download/v${version}"

download() {
  if ! curl --fail --location --silent --show-error --proto '=https' --proto-redir '=https' \
    --output "$temp_dir/$1" "$base_url/$1"; then
    printf 'error: failed to download %s from %s\n' "$1" "$base_url" >&2
    printf 'check your network connection and that release v%s exists for %s\n' \
      "$version" "$repository" >&2
    exit 1
  fi
}

download "$asset"
download "checksums-$target.txt"

expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$temp_dir/checksums-$target.txt")
if [ -z "$expected" ]; then
  printf 'error: no checksum entry for %s in release v%s\n' "$asset" "$version" >&2
  exit 1
fi

actual=$(sha256 "$temp_dir/$asset")
if [ "$actual" != "$expected" ]; then
  printf 'error: checksum verification failed for %s\n' "$asset" >&2
  printf 'expected: %s\n' "$expected" >&2
  printf 'actual:   %s\n' "$actual" >&2
  printf 'refusing to run a corrupted or tampered download\n' >&2
  exit 1
fi

if ! chmod 700 "$temp_dir/$asset"; then
  printf 'error: could not make the downloaded %s executable\n' "$asset" >&2
  exit 1
fi

status=0
if [ -t 1 ] && ( : </dev/tty ) 2>/dev/null; then
  # macOS cannot poll /dev/tty with kqueue, so a standalone binary given
  # /dev/tty as stdin aborts with "EINVAL: invalid argument, kqueue"
  # (oven-sh/bun#24158). Run it on a pseudo-terminal through script(1) instead,
  # which keeps stdin interactive without ever opening /dev/tty. Linux is
  # unaffected, and its script(1) takes different arguments.
  if [ "$os" = Darwin ] && command -v script >/dev/null 2>&1; then
    script -q /dev/null "$temp_dir/$asset" "$@" </dev/tty || status=$?
  else
    "$temp_dir/$asset" "$@" </dev/tty || status=$?
  fi
else
  "$temp_dir/$asset" "$@" || status=$?
fi
if [ "$status" -eq 126 ] || [ "$status" -eq 127 ]; then
  printf 'error: could not launch %s; check that the release asset matches this operating system and architecture\n' "$asset" >&2
  case "$target" in
    linux-*) printf 'Linux requires glibc 2.35 or newer; musl-based systems are not supported\n' >&2 ;;
  esac
fi
exit "$status"
