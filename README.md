# Global AGENTS.md

Shared `AGENTS.md` linked across supported AI coding harnesses.

`Global AGENTS.md` provides the `agent-links` CLI, which creates symlinks from
one shared `~/.config/agents/AGENTS.md` to the global instruction locations used
by supported AI coding harnesses. It detects installed harnesses and does not
overwrite existing files or unrelated symlinks.

## Run from GitHub

Run the one-shot standalone CLI without cloning or installing Bun/Node:

```sh
curl -fsSL https://raw.githubusercontent.com/johnnymayodev/Global-AGENTS.md/main/run.sh | bash
```

The runner downloads the version-pinned standalone executable and matching
SHA-256 checksum from the corresponding GitHub release, verifies the executable
before running it, and removes its private temporary directory on exit. It
preserves the current working directory and forwards all arguments to the CLI.
When a controlling terminal is available, it reconnects stdin from the terminal,
so the arrow-key/Space picker, plan review, and explicit confirmation work when
the shell script itself is piped to Bash. On macOS this uses a pseudo-terminal
through `script(1)`, because the embedded Bun runtime cannot poll `/dev/tty`
with kqueue. This is a one-shot run: it does not install the CLI, change `PATH`,
modify shell profiles, or use `sudo`.

The bootstrap requires `curl`, `mktemp`, a SHA-256 utility (`sha256sum` or
`shasum`), and the usual system shell utilities. It does not require Bun or
Node.js. Supported releases are macOS and glibc-based Linux on x64 and ARM64;
WSL uses the Linux assets. Linux builds target **glibc 2.35 or newer** (Ubuntu
22.04 baseline). musl-based distributions such as Alpine and native Windows are
not supported. The embedded Bun runtime makes each downloaded executable large.

The checksum detects a corrupted or mismatched download; because both files are
served by the same GitHub release, it is not an independent signature and does
not protect against a compromised release account or release itself. The
standalone runner is pinned to `v0.1.0`; the release workflow checks that its
version matches `package.json` before building and publishing assets.

## Usage

Bun is preferred; Node.js 22.18+, 24.x, and 26.x are also supported. No package
installation or `bun install` is needed.

1. Download or clone [Global AGENTS.md from GitHub](https://github.com/johnnymayodev/Global-AGENTS.md).
2. From the repository directory, run the local launcher (Bun is used when installed;
   otherwise it falls back to Node):

   ```sh
   ./agent-links
   ```

   Alternatively, invoke the TypeScript entry point directly:

   ```sh
   bun agent-links.ts
   # or
   node agent-links.ts
   ```

   If `~/.config/agents/AGENTS.md` does not exist, the CLI asks whether to copy
   an existing instructions file (enter its path) or create a blank one.

3. Use the arrow keys to move, Space to select harnesses, and Enter to review the
   plan; type `yes` to apply it. The one-shot runner opens this same picker by
   default.

## Standalone builds and releases

Build a standalone executable for the current machine (Bun 1.4+ is required only
for building):

```sh
sh build-standalone.sh
```

The optional target and output arguments are intended for builds on matching
native runners. For example:

```sh
sh build-standalone.sh bun-linux-x64 dist/agent-links-linux-x64
```

The release workflow builds the four supported
targets on native GitHub-hosted runners, creates per-platform SHA-256 checksum
files, and attaches both files to the version tag's GitHub release. Before a
release, update `package.json` and the pinned `version` in `run.sh` together,
then push the matching `vX.Y.Z` tag. The bootstrap will not work for a version
until its release assets exist.
