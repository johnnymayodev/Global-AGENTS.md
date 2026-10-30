# AGENTS.md

## What this project is

`agent-links` is a standalone Go CLI and full-screen TUI that symlinks one
shared configuration directory into the global locations used by supported AI
coding harnesses (Claude Code, Codex, Cursor, OpenCode, Pi, OMP, Grok, Hermes
Agent, Muse Code, and Antigravity).

The shared source lives under the user's home directory:

```text
~/<harness.SourceDirName>/
├── AGENTS.md
└── skills/<skill-name>/SKILL.md
```

`harness.SourceDirName` is currently `".global-agents"`, so normal installs use
`~/.global-agents`. Every path is resolved from the absolute `HOME` the CLI
receives.

Each instructions file and each skill directory is linked individually. The
tool never replaces a harness's whole configuration or skills directory, and it
never overwrites an existing file, directory, or unrelated symlink.

`README.md` is the user-facing source of truth for commands, the wizard, the
supported-harness table, exit codes, and installation. Keep it in sync with
behavior changes; this file is the contributor/agent guide.

The module path is `github.com/johnnymayodev/Global-AGENTS.md` (the repository
name itself contains `.md`). The entry point is `cmd/agent-links`.

## How it works

### Entry point and packages

```text
cmd/agent-links/main.go   calls cli.Run(os.Args[1:]) and exits with its code
internal/cli              argument dispatch, every flow, exit-code wrapping
internal/harness          environment/path resolution, harness definitions,
                          target-state classification, installation detection
internal/skill            shared-skill discovery and name validation
internal/plan             planning, applying, and rendering symlink mutations
internal/ui               plain-text Prompter and Bubble Tea wizard/matrix
internal/runner           tests for run.sh (test-only package)
internal/e2e              PTY tests for the built binary (test-only package)
```

The flow is: `cli.Run` builds a `harness.Context` from `HOME` and the
environment, dispatch selects a flow, the flow builds a plan in `internal/plan`,
the user reviews it through `internal/ui`, and applying the plan performs
exclusively `os.Symlink`/`os.Remove` operations on the planned destinations.

`cli.Options` injects the environment lookup, working directory, streams, and
interactive flag. Tests use `cli.RunWith` with `t.TempDir()` and never touch
the real home directory. Preserve that seam when adding commands.

### Harness resolution

`harness.BuildContext` resolves every path once from `HOME` (which must be set
to an absolute path) and these optional overrides:

- `CODEX_HOME` for Codex's instructions file (skills stay at `~/.agents/skills`)
- `HERMES_HOME` for both Hermes targets
- an absolute `XDG_CONFIG_HOME` for Muse Code (`<xdg>/muse`)
- `PI_CONFIG_DIR`, `OMP_PROFILE`/`PI_PROFILE`, and `PI_CODING_AGENT_DIR` for
  OMP/Pi agent directories

A harness counts as installed when its instructions target is already managed,
one of its executables is on `PATH`, or one of its install directories exists.
`--all` bypasses detection. Cursor has no global instructions file, so it is
omitted from instruction flows and appears only in the skills matrix.

### Plans and safety invariants

`harness.TargetState` classifies a destination as `managed` (symlink to the
canonical source), `symlink` (a different symlink), `path` (regular file or
directory), or `absent`. Plans are built from that state and every destination
is re-checked immediately before mutation.

Keep these invariants when changing plan or flow code:

- Never overwrite or remove an existing file, directory, or unrelated symlink.
  Conflicts abort the whole instruction/skill link plan before any change.
- Reconcile applies links before removals, so a failed creation leaves existing
  managed links in place.
- Unlink only removes links this tool manages; everything else is reported as
  left untouched.
- Instructions and skills are independent; skills mode does not create or
  require `AGENTS.md`.
- Instruction and skill actions are planned and reviewed separately, and the
  review text must come from the same plan object that `Apply` executes.
- Skills are discovered from immediate child directories (or symlinks to
  directories) containing a regular `SKILL.md`. Frontmatter is never parsed;
  each harness validates its own metadata.
- Claude reserves the skill names `synced` and the `anthropic-skills`
  namespace. New links with those names are unavailable for Claude only;
  existing managed links can still be selected for cleanup.
- Exit `0` for success/cancellation/no-op, `1` for runtime or validation errors,
  and `2` for invalid commands or arguments. `run.sh` adds its own codes; see
  the README.

### Interactive versus plain text

When stdin and stdout are both terminals, flows use the Bubble Tea wizard in
`internal/ui/tui.go` and the skills matrix in `internal/ui/skills.go`. Otherwise
they fall back to line prompts and read-only tables in
`internal/ui/plain.go`. `cli.Run` also opens `/dev/tty` as the TUI input to
avoid the macOS shutdown stall; `RunWith` leaves it nil for tests.

Removal always requires a terminal; without one, a non-empty removal plan is an
error. `--list` and `skills --list` are read-only and scriptable.

## Development

Requirements: Go 1.27.2 or newer (the `go` directive in `go.mod`).

Run the CLI from a checkout:

```sh
go run ./cmd/agent-links
```

That uses your real `HOME`, so it will read and can modify real harness
configuration if you confirm a plan. For manual testing, point `HOME` at a
throwaway directory:

```sh
tmp="$(mktemp -d)"
mkdir -p "$tmp/.global-agents"
HOME="$tmp" go run ./cmd/agent-links --all
```

Use `--list`, `skills --list`, and non-interactive input to inspect behavior
without applying changes. The code has no external runtime dependency.

## Testing

Run the same checks as CI from the repository root:

```sh
gofmt -l .          # must print nothing
go vet ./...
go test ./...
go build ./...
```

CI runs those steps on `ubuntu-latest` and `macos-15`. A separate job
cross-builds all four release targets, checks each binary's architecture with
`file`, and verifies the generated checksum files.

Test layers:

- `internal/cli` — byte-for-byte golden scenarios in `internal/cli/testdata`.
  Each scenario has `.stdout`, `.stderr`, and `.exit` files, with `{{HOME}}`
  standing in for the temporary home. The goldens were captured from the
  former TypeScript reference implementation, so treat them as compatibility
  expectations, not disposable snapshots. Regenerate with
  `go test ./internal/cli -update` only when the output change is intentional
  and reviewed.
- `internal/runner` — drives `run.sh` end to end with stub `curl`, `uname`, and
  downloaded-asset executables. No network or release is required.
- `internal/ui` — Bubble Tea model tests for navigation, selection, review,
  setup, and cancellation.
- `internal/e2e` — builds the binary and drives it through a real PTY (Python
  `pty.fork()` when available, otherwise `script(1)`; skips when neither
  exists). Covers picker, setup, review, signals, and terminal restoration.

Unit and model tests must stay hermetic: inject environment/streams through
`cli.Options`, use `t.TempDir()`, and do not read or write the developer's real
`HOME`.

## Build

`build.sh` cross-compiles any release target, defaulting to the host platform:

```sh
sh build.sh                          # dist/agent-links-<host target>
sh build.sh linux-x64                # one release target
sh build.sh linux-x64 /tmp/agent-links
```

Release targets are `darwin-arm64`, `darwin-x64`, `linux-arm64`, and
`linux-x64`. Output defaults to `dist/agent-links-<target>`; `dist/` is
gitignored. The script sets `CGO_ENABLED=0` (so Linux binaries are fully
static and run on glibc and musl), `-trimpath`, and `-ldflags "-s -w"`.

The binary does not embed or report a version. The release version is pinned by
`run.sh`'s `version=` line and the matching release tag/assets.

`run.sh` is the release-pinned one-shot bootstrap. It downloads
`agent-links-<target>` and `checksums-<target>.txt` from the GitHub release for
its `version=`, verifies SHA-256, caches them under
`${XDG_CACHE_HOME:-$HOME/.cache}/agent-links/v<version>/<target>/`, and runs
the cached binary with the caller's arguments, cwd, environment, and umask.

## Releases

Releases are tag-driven. GitHub Actions runs `.github/workflows/release.yml` on
any `v*` tag push.

Release procedure:

1. Create a `release/vX.Y.Z` branch from an up-to-date `main`.
2. Update `version=` in `run.sh` to `X.Y.Z` (no `v` prefix). This is the only
   version source.
3. Commit with the established subject:
   `build(release): bump the pinned run.sh version to X.Y.Z`
4. Open a PR, get it merged into `main`, and make sure CI is green.
5. From `main`, create a lightweight tag and push it:

   ```sh
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

6. Wait for the Release workflow to finish and confirm the GitHub release
   contains all four `agent-links-<target>` binaries and their
   `checksums-<target>.txt` files.

The `validate` job runs the CI checks and enforces that the pushed tag is a
semantic version matching `v` + `run.sh`'s `version=`, strictly newer than the
latest release, and on `main`; any mismatch fails before building. The `build`
job cross-compiles all four targets on one Linux runner, verifies each binary's
architecture and every per-target SHA-256 checksum file, and creates
build-provenance attestations. The `publish` job re-verifies the downloaded
artifacts, uploads them to a draft release created with `--generate-notes`,
checks the remote asset set, and then publishes it. Tags starting with `v0.` are
published as prereleases automatically; `v1.0.0` and later are normal releases.

The bootstrap does not work for a new version until that version's release
assets exist, so do not bump `run.sh` on `main` and leave the tag unpublished.
Do not add generated binaries or checksum files to the repository; they are
release artifacts only.
