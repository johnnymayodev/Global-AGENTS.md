# Global AGENTS.md

Shared `AGENTS.md` linked across supported AI coding harnesses.

`Global AGENTS.md` provides the `agent-links` CLI, which creates symlinks from
one shared `~/.config/agents/AGENTS.md` to the global instruction locations used
by supported AI coding harnesses. It detects installed harnesses and does not
overwrite existing files or unrelated symlinks.

## Run from GitHub

Run this project without cloning:

```sh
repo="johnnymayodev/Global-AGENTS.md"
curl -fsSL "https://raw.githubusercontent.com/${repo}/main/run.sh" |
  bash -s -- "$repo"
```

The runner downloads `agent-links.ts` from that repository's `main` branch into
a temporary directory, runs it with Bun, then removes the temporary file. It
reconnects stdin to `/dev/tty` when available so the interactive picker works
when the runner itself is piped to Bash. Bun and curl must be installed.

## Usage

Bun is required. No package installation or `bun install` is needed.

1. Download or clone [Global AGENTS.md from GitHub](https://github.com/johnnymayodev/Global-AGENTS.md).
2. From the repository directory, run:

   ```sh
   bun agent-links.ts
   ```

   If `~/.config/agents/AGENTS.md` does not exist, the CLI asks whether to copy
   an existing instructions file (enter its path) or create a blank one.

3. Use the arrow keys to move, Space to select harnesses, and Enter to apply.
