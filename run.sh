#!/bin/sh
set -eu

if [ "$#" -lt 1 ]; then
  printf 'error: pass the GitHub repository as OWNER/REPO\n' >&2
  exit 2
fi

repository=$1
shift

case "$repository" in
  */*) ;;
  *)
    printf 'error: expected GitHub repository in OWNER/REPO form\n' >&2
    exit 2
    ;;
esac

if ! command -v curl >/dev/null 2>&1; then
  printf 'error: curl is required to download agent-links from GitHub\n' >&2
  exit 127
fi

if command -v bun >/dev/null 2>&1; then
  runtime=bun
elif command -v node >/dev/null 2>&1; then
  runtime=node
else
  printf 'error: Bun 1.4+ or Node.js 22.18+, 24.x, or 26.x is required to run agent-links\n' >&2
  exit 127
fi

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/agent-links.XXXXXX")
trap 'rm -rf "$temp_dir"' 0

curl --fail --location --silent --show-error \
  "https://raw.githubusercontent.com/${repository}/main/agent-links.ts" \
  --output "$temp_dir/agent-links.ts"

run_agent_links() {
  if [ "$runtime" = bun ]; then
    bun run "$temp_dir/agent-links.ts" "$@"
  else
    node "$temp_dir/agent-links.ts" "$@"
  fi
}

if [ -t 1 ] && [ -r /dev/tty ]; then
  run_agent_links "$@" </dev/tty
else
  run_agent_links "$@"
fi
