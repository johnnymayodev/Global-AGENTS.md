#!/bin/sh
set -eu

# Builds a standalone executable with `bun build --compile`.
#
# Usage:
#   sh build-standalone.sh [target] [outfile]
#
# target is a Bun compile target (default: this machine's platform), e.g.
# bun-darwin-arm64, bun-linux-x64. outfile defaults to
# dist/agent-links-<os>-<arch>.
#
# --no-compile-autoload-dotenv / --no-compile-autoload-bunfig keep the
# executable from reading .env or bunfig.toml in the directory it runs from.

script_dir="$(CDPATH= cd -P "$(dirname "$0")" && pwd)"

if ! command -v bun >/dev/null 2>&1; then
  printf 'error: Bun is required to build the standalone agent-links executable\n' >&2
  exit 127
fi

target=${1:-"bun-$(bun -p 'process.platform + "-" + process.arch')"}

case "$target" in
  bun-darwin-x64 | bun-darwin-arm64 | bun-linux-x64 | bun-linux-arm64) ;;
  *)
    printf 'error: unsupported target: %s\n' "$target" >&2
    printf 'supported: bun-darwin-x64, bun-darwin-arm64, bun-linux-x64, bun-linux-arm64\n' >&2
    exit 2
    ;;
esac

arch=${target#bun-}
outfile=${2:-"$script_dir/dist/agent-links-$arch"}

mkdir -p "$(dirname "$outfile")"

exec bun build --compile \
  --no-compile-autoload-dotenv \
  --no-compile-autoload-bunfig \
  --target="$target" \
  --outfile="$outfile" \
  "$script_dir/cli.ts"
