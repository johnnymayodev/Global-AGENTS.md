#!/bin/sh
set -eu

script_dir="$(CDPATH= cd -P "$(dirname "$0")" && pwd)"
compiler="$script_dir/node_modules/typescript/bin/tsc"

if command -v bun >/dev/null 2>&1; then
  exec bun "$compiler" --project "$script_dir/tsconfig.build.json"
fi

if command -v node >/dev/null 2>&1; then
  exec node "$compiler" --project "$script_dir/tsconfig.build.json"
fi

printf 'error: Bun or Node.js is required to build agent-links\n' >&2
exit 127
