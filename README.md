# Global AGENTS.md

Keep shared personal instructions in `~/.config/agents/AGENTS.md`, then use
`agent-links` to link that file into the supported harnesses' global
instruction locations.

```sh
mkdir -p ~/.config/agents
$EDITOR ~/.config/agents/AGENTS.md
./agent-links link
```

The script links to:

- OpenCode: `~/.config/opencode/AGENTS.md`
- Codex: `${CODEX_HOME:-~/.codex}/AGENTS.md`
- Pi: `~/.pi/agent/AGENTS.md`
- OMP: `~/.omp/agent/AGENTS.md` by default
- Hermes Agent: `${HERMES_HOME:-~/.hermes}/AGENTS.md`
- Claude Code: `~/.claude/CLAUDE.md`

Set `CODEX_HOME` or `HERMES_HOME` when those tools use a non-default home. OMP
profile and directory overrides are honored through `OMP_PROFILE` (or legacy
`PI_PROFILE`), `PI_CONFIG_DIR`, and `PI_CODING_AGENT_DIR`. `link` is safe to
rerun; it checks all destinations first and refuses to overwrite existing
files or unrelated symlinks. `unlink` removes only symlinks whose stored target
exactly matches the canonical file path and leaves everything else alone.

```sh
./agent-links unlink
```

Codex prefers a non-empty `AGENTS.override.md` over `AGENTS.md`. Claude Code's
Cowork desktop mode may also skip a `CLAUDE.md` symlink that points outside the
session workspace; the Claude Code link is intended for local CLI sessions.

Hermes Agent's current documentation guarantees global `SOUL.md` loading from
`HERMES_HOME`, but documents `AGENTS.md` as project context. The script creates
the conventional `HERMES_HOME/AGENTS.md` link, but Hermes will only load it
globally in versions that support that behavior.
