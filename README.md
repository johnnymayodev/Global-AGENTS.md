# Global AGENTS.md

`agent-links` is a Go CLI and TUI that symlinks one shared config directory into
the global locations of supported AI coding harnesses:

```text
~/.global-agents/
├── AGENTS.md
└── skills/<skill-name>/SKILL.md
```

Links are per file and per skill. Existing files, directories, and unrelated
symlinks are never overwritten.

## Run

One-shot, no install, no `PATH` changes:

```sh
curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
```

Swap `main` for a tag or commit SHA to pin the script.

## Install

No Go required. Download the release binary into `~/.local/bin`:

```sh
tag=v0.5.3   # latest release tag
```

```sh
target=darwin-arm64   # or darwin-x64, linux-x64, linux-arm64
```

```sh
mkdir -p ~/.local/bin
```

```sh
curl -fsSL -o ~/.local/bin/agent-links \
  "https://github.com/johnnymayodev/Global-AGENTS.md/releases/download/$tag/agent-links-$target"
```

```sh
chmod +x ~/.local/bin/agent-links
```

With Go 1.27.2+, install from source instead:

```sh
GOBIN=~/.local/bin go install github.com/johnnymayodev/Global-AGENTS.md/cmd/agent-links@latest
```

Or build from a checkout:

```sh
git clone https://github.com/johnnymayodev/Global-AGENTS.md.git
```

```sh
cd Global-AGENTS.md
```

```sh
go build -o ~/.local/bin/agent-links ./cmd/agent-links
```

Make sure `~/.local/bin` is on `PATH`, then check `agent-links --version`.
Cross-build any release target with `sh build.sh <target> [outfile]`:
`darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`. Linux builds are
static (glibc and musl). No native Windows.

## Usage

```text
agent-links                          choose instructions or skills, then harnesses
agent-links --all                    same, including undetected harnesses
agent-links --list [--all]           print instruction targets
agent-links link                     link instructions to all harnesses
agent-links unlink                   remove managed instruction links
agent-links skills [--all]           skills matrix for detected (or all) harnesses
agent-links skills --list [--all]    print the skills matrix
agent-links skills link <skill>      link one skill
agent-links skills unlink <skill>    unlink one skill
agent-links --help                   print usage
agent-links --version                print the version
```

- In a terminal, `agent-links` opens a full-screen wizard: mode → setup (first
  run only) → selection → review. `enter` applies, `esc` goes back, `ctrl+c`
  cancels.
- `--list` and `skills --list` are read-only and scriptable.
- Removing links requires a terminal.
- Instructions and skills are independent; skills mode never creates
  `AGENTS.md`.
- `HOME` must be absolute.

| Screen        | Keys                                                                       |
| ------------- | -------------------------------------------------------------------------- |
| Mode menu     | `↑`/`↓` or `j`/`k`, `enter`, `1` instructions, `2` skills, `q` cancel      |
| Setup menu    | `↑`/`↓` or `j`/`k`, `enter`, `1` copy, `2` blank, `esc` back              |
| Harness picker | `↑`/`↓` or `j`/`k`, `space` toggle, `a` all, `n` none, `enter`, `?` help  |
| Skills matrix | `↑`/`↓` rows, `←`/`→` harnesses, `space` toggle, `a` all, `n` clear, `enter`|
| Review        | `enter` apply, `esc` back, `↑`/`↓`/`pgup`/`pgdown` scroll, `ctrl+c` cancel |

Without a terminal, interactive commands fall back to plain-text prompts and
require a typed `yes` to apply.

## Supported harnesses

| Harness      | Instructions                                             | Skills                         |
| ------------ | -------------------------------------------------------- | ------------------------------ |
| Claude Code  | `~/.claude/CLAUDE.md`                                     | `~/.claude/skills`           |
| Codex        | `$CODEX_HOME/AGENTS.md` (default `~/.codex/AGENTS.md`)    | `~/.agents/skills`           |
| Cursor       | —                                                         | `~/.cursor/skills`           |
| OpenCode     | `~/.config/opencode/AGENTS.md`                            | `~/.config/opencode/skills`  |
| Pi           | `~/.pi/agent/AGENTS.md`                                   | `~/.pi/agent/skills`         |
| omp          | `<agent dir>/AGENTS.md` (default `~/.omp/agent/...`)      | `<agent dir>/skills`         |
| Grok         | `~/.grok/AGENTS.md`                                       | `~/.grok/skills`             |
| Hermes Agent | `$HERMES_HOME/AGENTS.md` (default `~/.hermes/AGENTS.md`)  | `$HERMES_HOME/skills`        |
| Muse Code    | `$XDG_CONFIG_HOME/muse/AGENTS.md`                         | `$XDG_CONFIG_HOME/muse/skills` |
| Antigravity  | `~/.gemini/AGENTS.md`                                     | `~/.gemini/config/skills`    |

- Cursor has no global instructions file; it appears only in the skills matrix.
- OMP follows `PI_CONFIG_DIR`, `OMP_PROFILE`/`PI_PROFILE`, and
  `PI_CODING_AGENT_DIR`; Hermes follows `HERMES_HOME`; Muse Code follows an
  absolute `XDG_CONFIG_HOME`.
- Claude reserves the skill names `synced` and the `anthropic-skills` namespace.
- A non-empty `$CODEX_HOME/AGENTS.override.md` takes precedence over `AGENTS.md`.

## Skills

- A skill is an immediate child directory (or symlink to one) containing a
  regular `SKILL.md`; the basename is the name.
- The whole directory is linked, so scripts, references, and assets come along.
- Frontmatter is not parsed; each harness validates its own metadata.
- Conflict cells (`!`) are never overwritten; unavailable pairs (`–`) show a
  reason.
- Links are created before any removal, so a failed create leaves existing links
  in place.

## Exit codes

| Code | Meaning                                                                      |
| ---- | ---------------------------------------------------------------------------- |
| `0`  | Success, cancellation, declined confirmation, or nothing to do               |
| `1`  | Runtime or validation error (bad `HOME`, malformed source, conflict, no TTY) |
| `2`  | Invalid command or arguments                                                 |

The runner passes these through and adds `2` (unsupported platform), `126`/`127`
(cannot launch or missing utility), and `129`/`130`/`143` (signal).

## Development

```sh
git clone https://github.com/johnnymayodev/Global-AGENTS.md.git
cd Global-AGENTS.md
go run ./cmd/agent-links

gofmt -l .
go vet ./...
go test ./...
go build ./...
```

Tests are hermetic; point `HOME` at a temp directory for manual runs.

## Releases and security

- Releases are tag-driven; bump `version=` in `run.sh` and push the matching
  `vX.Y.Z` tag.
- Verify any asset with
  `gh attestation verify <file> --repo johnnymayodev/Global-AGENTS.md`.
- See [`SECURITY.md`](SECURITY.md) for reporting and what checksums prove.
