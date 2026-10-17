# Global AGENTS.md

Shared `AGENTS.md` linked across supported AI coding harnesses.

`Global AGENTS.md` provides the `agent-links` CLI, a standalone Go binary that
creates symlinks from one shared `~/.config/agents/AGENTS.md` to the global
instruction locations used by supported AI coding harnesses. It detects
installed harnesses and does not overwrite existing files or unrelated symlinks.
Selection, plan review, and confirmation run in an interactive terminal UI built
with Bubble Tea.

## Run from GitHub

Run the one-shot standalone CLI without cloning, installing a runtime, or
building anything:

```sh
curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
```

The runner downloads the version-pinned standalone executable and matching
SHA-256 checksum from the corresponding GitHub release, verifies the executable
before running it, and removes its private temporary directory on exit. It
preserves the current working directory and forwards all arguments to the CLI.
When a controlling terminal is available, it reconnects stdin from the terminal,
so the arrow-key/Space picker, plan review, and explicit confirmation work when
the shell script itself is piped to Bash. This is a one-shot run: it does not
install the CLI, change `PATH`, modify shell profiles, or use `sudo`.

The bootstrap requires `curl`, `mktemp`, a SHA-256 utility (`sha256sum` or
`shasum`), and the usual system shell utilities; no language runtime is
required. Supported releases are macOS and Linux on x64 and ARM64; the static
Linux binaries also run on musl-based distributions such as Alpine, and WSL uses
the Linux assets. Native Windows is not supported.

The checksum detects a corrupted or mismatched download; because both files are
served by the same GitHub release, it is not an independent signature and does
not protect against a compromised release account or release itself. The
standalone runner is pinned to `v0.2.0`; the release workflow checks that the tag
matches the pinned `version` in `run.sh` before building and publishing assets.

## Install or build from source

Go 1.26 or newer (the `go` directive in `go.mod`) is required to install or
build the CLI:

```sh
go install github.com/johnnymayodev/Global-AGENTS.md/cmd/agent-links@latest
```

To build from a checkout instead, `sh build.sh [target] [outfile]`
cross-compiles any of the four release targets and defaults to the host
platform:

```sh
sh build.sh                          # dist/agent-links-<host target>
sh build.sh linux-x64                # any release target
sh build.sh linux-x64 /tmp/agent-links
```

The release targets are `darwin-arm64`, `darwin-x64`, `linux-arm64`, and
`linux-x64`. `CGO_ENABLED=0` makes the Linux builds fully static, so they run on
both glibc and musl systems. The npm package `global-agents-md` and its
`runAgentLinks` export are retired: there is no JavaScript API, and programmatic
consumers exec the `agent-links` binary.

## Usage

On first run, if `~/.config/agents/AGENTS.md` does not exist, the CLI asks
whether to copy an existing instructions file (enter its path) or create a blank
one. Use the arrow keys to move and Space to select harnesses; Enter shows the
plan, and Enter again applies it. `esc` steps back, `ctrl+c` cancels, and `?`
shows the full key help. The one-shot runner opens this same picker by default.

## Releases

The release workflow cross-compiles the four supported targets on one Linux
runner, creates per-platform SHA-256 checksum files, and attaches both files to
the version tag's GitHub release. Before a release, update the pinned `version`
in `run.sh` and push the matching `vX.Y.Z` tag. The bootstrap will not work for a
version until its release assets exist.
