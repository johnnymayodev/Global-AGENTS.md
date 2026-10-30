#!/bin/sh
set -eu

# Cross-compiles the agent-links binary.
#
# Usage:
#   sh build.sh [target] [outfile]
#
# target is one of the four release targets (default: this machine's platform):
# darwin-x64, darwin-arm64, linux-x64, linux-arm64. outfile defaults to
# dist/agent-links-<target> in the repository.
#
# CGO_ENABLED=0 produces fully static Linux binaries, so they do not depend on
# a glibc baseline and also run on musl-based distributions; the tool does no
# DNS or user lookup, so static linking has no behavioral cost.

script_dir="$(CDPATH= cd -P "$(dirname "$0")" && pwd)"

if ! command -v go >/dev/null 2>&1; then
  printf 'error: Go is required to build agent-links\n' >&2
  exit 127
fi

host_arch=$(go env GOHOSTARCH)
case "$host_arch" in
  amd64) host_arch=x64 ;;
esac
host_target="$(go env GOHOSTOS)-$host_arch"

target=${1:-"$host_target"}

case "$target" in
  darwin-x64 | darwin-arm64 | linux-x64 | linux-arm64) ;;
  *)
    printf 'error: unsupported target: %s\n' "$target" >&2
    printf 'supported: darwin-x64, darwin-arm64, linux-x64, linux-arm64\n' >&2
    exit 2
    ;;
esac

os=${target%-*}
case "${target#*-}" in
  x64) goarch=amd64 ;;
  *) goarch=${target#*-} ;;
esac
outfile=${2:-"$script_dir/dist/agent-links-$target"}
case "$outfile" in
  /*) ;;
  *) outfile="$PWD/$outfile" ;;
esac

mkdir -p "$(dirname "$outfile")"
cd "$script_dir"

# run.sh's version= line is the release version source. Stamping it here lets
# release binaries report which version they came from.
version=$(sed -n 's/^version=\([^[:space:]]*\)$/\1/p' "$script_dir/run.sh")
if [ -z "$version" ]; then
  printf 'error: could not read version= from run.sh\n' >&2
  exit 1
fi

CGO_ENABLED=0 GOOS="$os" GOARCH="$goarch" exec go build \
  -trimpath \
  -ldflags "-s -w -X github.com/johnnymayodev/Global-AGENTS.md/internal/cli.Version=$version" \
  -o "$outfile" \
  ./cmd/agent-links
