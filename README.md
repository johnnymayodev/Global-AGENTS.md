# Global AGENTS.md

Symlink one shared `~/.config/agents/AGENTS.md` into the global instruction locations of supported AI coding harnesses.

## Usage

```sh
curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
```

## Supported Harnesses

| Harness      | Global instructions file                                   | Executable        |
| ------------ | ---------------------------------------------------------- | ----------------- |
| Claude Code  | `~/.claude/CLAUDE.md`                                      | `claude`          |
| Codex        | `$CODEX_HOME/AGENTS.md` (default `~/.codex/AGENTS.md`)     | `codex`           |
| Hermes Agent | `$HERMES_HOME/AGENTS.md` (default `~/.hermes/AGENTS.md`)   | `hermes`          |
| OpenCode     | `~/.config/opencode/AGENTS.md`                             | `opencode`        |
| Pi           | `~/.pi/agent/AGENTS.md`                                    | `pi`              |
| OMP          | `<agent dir>/AGENTS.md` (default `~/.omp/agent/AGENTS.md`) | `omp`, `oh-my-pi` |

## Development

```
git clone https://github.com/johnnymayodev/Global-AGENTS.md.git
```

Run the CLI directly from the repository root with Go 1.27.2 or newer:

```sh
go run ./cmd/agent-links
```
