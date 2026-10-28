# Global AGENTS.md

`agent-links` is a standalone Go CLI and full-screen TUI that symlinks one
shared configuration directory into the global locations of supported AI coding
harnesses:

- `~/.config/agents/AGENTS.md` holds shared instructions.
- `~/.config/agents/skills/<skill-name>/` holds portable Agent Skills.

Each file or skill directory is linked individually. The tool never replaces a
harness's whole configuration or skills directory, and never overwrites an
existing file, directory, or unrelated symlink. It detects installed harnesses,
previews planned changes, and asks for confirmation before applying them.

## Install

### One-shot runner (recommended)

Run the release-pinned bootstrap without cloning, installing a runtime, or
changing `PATH`:

```sh
curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
```

The runner downloads `agent-links-<target>` and its matching
`checksums-<target>.txt` from the version pinned in `run.sh` (currently
`v0.4.0`), verifies the executable's SHA-256 checksum, and caches both under
`${XDG_CACHE_HOME:-$HOME/.cache}/agent-links/v<version>/<target>/`. The first
run downloads both files; later runs re-verify the cached executable's SHA-256
and need no network. It forwards your arguments, working directory,
environment, and umask to the CLI. When stdout is a terminal it reconnects stdin
from `/dev/tty`, so the full-screen wizard still works when the script itself is
piped to a shell. The binary also opens the controlling terminal for its own
input, which avoids a macOS shutdown stall; the redirect remains for
compatibility with older released binaries.

The per-version cache directory is private to your user (`0700`; the executable
is `0700` and the checksum file `0600`). The runner rejects symlinked version
directories, symlinked cache entries, and group/world-writable managed
directories, and falls back to a fresh temporary directory when the cache root
is unusable. A warm cache needs neither
`curl` nor `mktemp`; every run needs a SHA-256 utility (`sha256sum` or `shasum`)
and the usual POSIX shell utilities. Clear the cache with
`rm -rf "${XDG_CACHE_HOME:-$HOME/.cache}/agent-links"`.

This is a one-shot run: it does not install the CLI, change `PATH` or shell
profiles, or use `sudo`. On a cache miss it requires `curl` and `mktemp` in
addition to the checksum utility.

The bootstrap requires that the pinned release and its assets already exist.
The cached checksum detects corruption or a mismatched download; because both
files come from the same GitHub release and the cached checksum sits next to the
binary, it is not an independent signature. A local writer who can replace both
cache files can replace the binary, so the private cache directory and its
ownership checks are the trust boundary.

### Install or build from source

Go 1.27.2 or newer (the `go` directive in `go.mod`) is required to install or
build from source:

```sh
go install github.com/johnnymayodev/Global-AGENTS.md/cmd/agent-links@latest
```

This installs a binary named `agent-links`.

To build from a checkout, `sh build.sh [target] [outfile]` cross-compiles any
release target and defaults to the host platform:

```sh
sh build.sh                          # dist/agent-links-<host target>
sh build.sh linux-x64                # any release target
sh build.sh linux-x64 /tmp/agent-links
```

The release targets are `darwin-arm64`, `darwin-x64`, `linux-arm64`, and
`linux-x64`. `CGO_ENABLED=0` makes the Linux builds fully static, so they run on
both glibc and musl systems.

### Supported platforms

macOS and Linux on x64 and ARM64 are supported. The static Linux binaries also
run on musl-based distributions such as Alpine, and WSL uses the Linux assets.
Native Windows is not supported.

## Usage

With no arguments in a terminal, `agent-links` opens the interactive wizard. It
first asks whether to manage global instructions or shared skills, then shows
the detected harnesses. `--all` includes every supported harness, whether or not
it looks installed.

On first run, if `~/.config/agents/AGENTS.md` does not exist, the instructions
flow offers to copy an existing instructions file or create a blank one. Skills
mode does not require or create `AGENTS.md`, and the two modes are planned and
linked independently.

### Commands

```text
agent-links                          choose instructions or skills, then harnesses
agent-links --all                    choose instructions or skills, then all harnesses
agent-links --list [--all]           print the detected (or all) instruction targets
agent-links link                     preview and confirm linking all supported harnesses
agent-links unlink                   preview and confirm removal of managed links
agent-links skills [--all]           manage shared skills for detected (or all) harnesses
agent-links skills --list [--all]    print the shared skills matrix
agent-links skills link <skill>      preview and confirm linking one skill
agent-links skills unlink <skill>    preview and confirm unlinking one skill
```

- `agent-links` and `agent-links --all` run the wizard.
- `--list` prints the read-only instructions table. It does not prompt for or
  create the canonical file.
- `link` links the shared instructions file into every supported harness.
- `unlink` removes only managed instruction links; unrelated entries are left
  untouched.
- `skills` opens the interactive skill-by-harness matrix; `skills --all` includes
  every supported harness.
- `skills --list [--all]` prints the read-only matrix.
- `skills link <skill>` links one existing shared skill into every supported
  harness where the pair is available. Statically unavailable pairs are
  skipped with a reason. A missing skill or a destination that already holds a
  file, directory, or unrelated symlink aborts the whole plan before any change.
- `skills unlink <skill>` removes that skill's managed links. The source skill
  does not have to exist. Absolute names, path separators, `.`, and `..` are
  rejected.

Instructions and skills are independent: skills mode does not require or create
`AGENTS.md`, and each mode's links are planned separately. `HOME` must be set to
an absolute path; the CLI resolves every target from it.

## Interactive wizard

In a terminal, `agent-links` runs one full-screen alternate-screen wizard built
with Bubble Tea. Every step shares the same framed layout and key help; the
selection steps add a status line.

1. **Mode menu** chooses global instructions, global skills, or cancel.
2. **First-run setup** (instructions only, when `AGENTS.md` is missing) offers to
   copy an existing file or create a blank one. Once the file exists, backing
   out and choosing instructions again skips setup.
3. **Selection** shows the harness picker or the skills matrix. Existing managed
   links start selected; absent destinations start unselected. An empty mode
   stays on screen with a hint and can be left with `esc`.
4. **Review** shows exactly what will be created and removed. Pressing `enter`
   applies that plan on a single keypress, `esc` returns to the previous step,
   and `ctrl+c` cancels without changes. A selection that would change nothing
   exits with `No changes made.`, or `No links were changed.` when setup created
   the instructions file in the same run.

In the wizard opened from the mode menu, `esc` means **back**: the setup menu,
the harness picker, the skills matrix, and a recoverable error screen all return
to the mode menu, and review returns to the selection. `ctrl+c` always cancels
the whole run. A broken instructions or skills source shows a recoverable error
screen for that mode only; `esc` returns to the mode menu, where the other mode
still works. Cancelling from that error screen exits `0`; plain-text commands
still exit `1` for a malformed source.

| Step         | Keys                                                                                                 |
| ------------ | ---------------------------------------------------------------------------------------------------- |
| Mode menu    | `↑`/`↓` or `j`/`k` move, `enter` choose, `1` instructions, `2` skills, `q`/`esc`/`ctrl+c` cancel     |
| Setup menu   | `↑`/`↓` or `j`/`k` move, `enter` choose, `1` copy, `2` blank, `esc` back, `q`/`ctrl+c` cancel       |
| Setup path   | Type the path, `enter` continue when valid, `esc` back to the menu, `ctrl+c` cancel                  |
| Harness picker | `↑`/`↓` or `j`/`k` move, `space` toggle, `a` all, `n` none, `enter` review, `?` help, `esc` back to the mode menu |
| Skills matrix | `↑`/`↓` or `j`/`k` rows, `←`/`→` or `h`/`l` harnesses, `space` toggle a cell, `a` select all available, `n` clear, `enter` review, `?` help, `esc` back to the mode menu |
| Review       | `enter` apply, `esc` back, `↑`/`↓` or `j`/`k` scroll, `pgup`/`pgdown` scroll by half a page, `?` help, `ctrl+c` cancel |

The standalone review used by `link` and `unlink` shares the same view; because
there is no selection step to return to, `esc` cancels the whole run there. The
standalone `skills` matrix and the standalone setup used by `link` likewise have
no previous screen, so `esc` cancels there. The skills matrix scrolls rows and
columns on small terminals, and the review scrolls when the plan is long. Bubble
Tea restores the alternate screen and cursor on normal exits, cancellation,
`ctrl+c`, `SIGINT`, `SIGTERM`, and recovered panics.

### Without a terminal

When stdin or stdout is not a terminal, the interactive commands stay in
plain-text mode. `agent-links` asks `1`/`2`/`q` for the mode, prints the
applicable table, and reports that interactive selection needs a terminal.
`link` and `skills link` can still be driven by piped input and require a typed
`yes` for confirmation, while the plain setup flow accepts piped `1`/`2`/`q`
choices. Removing links requires a terminal; without one, a non-empty removal
plan exits with an error. `--list` and `skills --list` are read-only and remain
scriptable.

## Exit codes

| Code | Meaning                                                                                                                        |
| ---- | ------------------------------------------------------------------------------------------------------------------------------ |
| `0`  | Success. Also cancellation, a declined confirmation, a plan with nothing to do, and cancelling from the wizard's recoverable error screen. |
| `1`  | Runtime or validation error: `HOME` missing or not absolute, malformed canonical file, refused overwrite, missing skill, destination conflict, non-empty removal without a terminal, or an I/O error. |
| `2`  | Invalid command or argument combination; usage is printed.                                                                     |

The one-shot runner passes the CLI's exit code through and adds runner-specific
results:

| Code    | Meaning                                                              |
| ------- | -------------------------------------------------------------------- |
| `2`     | Unsupported operating system or architecture.                        |
| `127`   | A SHA-256 utility is missing, or `curl`/`mktemp` is missing when a download is needed. |
| `1`     | Temporary-directory, download, checksum, missing-checksum-entry, or `chmod` failure. |
| `126`/`127` | The downloaded asset could not be launched.                      |
| `129`/`130`/`143` | Interrupted by `HUP`, `INT`, or `TERM`.                     |

## Canonical layout

```text
~/.config/agents/
├── AGENTS.md
└── skills/
    ├── git-release/
    │   ├── SKILL.md
    │   ├── scripts/
    │   └── references/
    └── pdf-tools/
        ├── SKILL.md
        └── assets/
```

A skill is any immediate child directory, or symlink to a directory, that
contains a regular `SKILL.md`. The directory basename is the skill name. The
whole directory is linked, so scripts, references, assets, and other relative
files stay available. The tool does not parse frontmatter; each harness
validates its own skill metadata.

If the canonical skills directory is missing or has no qualifying children,
the tool explains the layout and makes no changes. It does not create, import,
or copy skills.

## Supported harnesses

| Harness      | Global instructions file                                   | Global skills directory                          | Executable        |
| ------------ | ---------------------------------------------------------- | ------------------------------------------------ | ----------------- |
| Claude Code  | `~/.claude/CLAUDE.md`                                      | `~/.claude/skills`                               | `claude`          |
| Codex        | `$CODEX_HOME/AGENTS.md` (default `~/.codex/AGENTS.md`)     | `~/.agents/skills`                               | `codex`           |
| Hermes Agent | `$HERMES_HOME/AGENTS.md` (default `~/.hermes/AGENTS.md`)   | `$HERMES_HOME/skills`                            | `hermes`          |
| OpenCode     | `~/.config/opencode/AGENTS.md`                             | `~/.config/opencode/skills`                      | `opencode`        |
| Pi           | `~/.pi/agent/AGENTS.md`                                    | `~/.pi/agent/skills`                             | `pi`              |
| OMP          | `<agent dir>/AGENTS.md` (default `~/.omp/agent/AGENTS.md`) | `<agent dir>/skills`                             | `omp`, `oh-my-pi` |

Codex's documented user skills location is `~/.agents/skills`; `CODEX_HOME`
continues to affect only its instructions file. OMP follows `PI_CONFIG_DIR`,
`OMP_PROFILE`/`PI_PROFILE`, and `PI_CODING_AGENT_DIR` for both targets, and
Hermes follows `HERMES_HOME` for both.

Two harness-specific notes can appear after linking or reconciliation runs:

- If `$CODEX_HOME/AGENTS.override.md` exists and is non-empty, Codex prefers it
  over `AGENTS.md`.
- Hermes global `AGENTS.md` loading depends on the Hermes version; current
  Hermes documentation guarantees a global `SOUL.md`, not `AGENTS.md`.

## Skills matrix

The interactive matrix has one row per skill and one column per visible
harness. Each cell is independent: linking a skill to one harness never links
it to another.

- `↑`/`↓` or `j`/`k` moves between skills; `←`/`→` or `h`/`l` moves between
  harnesses.
- `space` toggles the focused cell, `a` selects every available cell, and `n`
  clears the selection.
- `enter` reviews the exact plan, `esc` returns to the matrix, and `?` shows
  all keys.
- Rows and columns scroll on small terminals, and the review scrolls when the
  plan is long.
- Existing managed links start selected. Absent destinations start unselected.

The read-only matrix uses the same symbols: `✓` managed, `·` absent, `!`
conflict, and `–` unavailable.

Destinations that already hold a file, directory, or unrelated symlink are
conflict cells (`!`) and cannot be toggled; they are never overwritten. Static
skill/harness incompatibilities are shown as unavailable (`–`) with a reason.

Claude Code reserves the name `synced` and the `anthropic-skills` namespace. A
new link with either name is blocked for Claude only. An existing managed link
with a reserved name can still be selected for cleanup.

Applying a plan creates all approved links before removing any managed link. If
a creation fails, no managed links are removed. Every destination is
re-checked immediately before it is changed.

## Cross-discovery

The matrix manages physical destinations, not effective visibility. OpenCode,
Codex, and Pi can also read shared locations such as `~/.agents/skills`, and
OpenCode can read Claude-compatible locations. A skill linked for one harness
may therefore be discovered by another. The tool does not create duplicate
compatibility links or edit harness settings to prevent this.

Other files and harness-specific skills in a skills directory are left alone;
the tool only manages the individual names it links.

## Development

```sh
git clone https://github.com/johnnymayodev/Global-AGENTS.md.git
cd Global-AGENTS.md
go run ./cmd/agent-links
```

Use Go 1.27.2 or newer. The project uses Bubble Tea v2, Bubbles v2, and Lip
Gloss v2 for the TUI, and `github.com/charmbracelet/x/term` for terminal
detection.

Run the same checks as CI:

```sh
gofmt -l .
go vet ./...
go test ./...       # unit, golden, runner, and PTY end-to-end tests
go build ./...
```

The PTY end-to-end tests use `python3` or `script(1)` and skip when neither is
available. CI builds all four release targets with `sh build.sh <target>`.

## Releases

The release workflow cross-compiles the four supported targets on one Linux
runner, creates per-platform SHA-256 checksum files, and attaches both files to
the version tag's GitHub release. `v0.x` tags are published as prereleases.

Before a release, update the pinned `version=` in `run.sh` and push the matching
`vX.Y.Z` tag. The workflow rejects a tag that does not match `run.sh`, and the
bootstrap does not work for a version until its release assets exist.
