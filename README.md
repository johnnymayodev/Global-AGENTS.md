# Global AGENTS.md

Symlink one shared `~/.config/agents/` directory into the global configuration
locations of supported AI coding harnesses:

- `~/.config/agents/AGENTS.md` holds shared instructions.
- `~/.config/agents/skills/<skill-name>/` holds portable Agent Skills.

Each file or skill directory is linked individually. The tool never replaces a
harness's whole configuration or skills directory, and never overwrites an
existing file, directory, or unrelated symlink.

## Usage

```sh
curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
```

Without arguments the tool asks whether to manage global instructions or
global skills, then shows the detected harnesses. `--all` includes every
supported harness, whether or not it looks installed.

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

Instructions and skills are independent: skills mode does not require or create
`AGENTS.md`, and instructions mode does not inspect the skills directory.

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

```
git clone https://github.com/johnnymayodev/Global-AGENTS.md.git
```

Run the CLI directly from the repository root with Go 1.27.2 or newer:

```sh
go run ./cmd/agent-links
```
