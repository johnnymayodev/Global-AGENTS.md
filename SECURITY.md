# Security policy

## Supported versions

Only the latest `v0.x` release is supported. Every `v0.x` tag is published as a
prerelease by policy, so fixes land in a new version rather than in a backport.

## Reporting a vulnerability

Report privately through GitHub security advisories: open the repository's
**Security** tab and choose **Report a vulnerability**. Please do not open a
public issue first. No email address or other private contact channel is
published.

This is a small project without a bug-bounty, CVE, or disclosure-SLA program.
Reports are handled on a best-effort basis.

## Trust model

- **Checksums are integrity checks, not signatures.** `run.sh` verifies the
  downloaded `agent-links-<target>` against the matching
  `checksums-<target>.txt` from the same GitHub release before running the
  binary. Both files come from the same source, and the cached checksum sits
  next to the cached binary, so whoever can replace one can usually replace the
  other.
- **Artifact attestations bind assets to this repository.** The release
  workflow attests the four binaries, the four checksum files, and the staged
  `run.sh`, tying them to the release workflow and the tagged commit. Verify
  with `gh attestation verify <asset> --repo johnnymayodev/Global-AGENTS.md`;
  see [Verify a release](README.md#verify-a-release). Releases published before
  attestations were added, and older releases without a `run.sh` asset, have no
  attestation.
- **The cache is a local-user boundary.** `run.sh` forces the per-version cache
  directory `0700`, the cached binary `0700`, and the cached checksum file
  `0600`, and rejects symlinked cache entries and group- or world-writable
  managed directories. That protects against other local users, not against a
  compromised account: anything running as you can replace the cached pair.
- **`main/run.sh` is mutable.** The convenience install command executes
  whatever script is on `main` at that moment, before any asset is verified.
  Pinning a tag or commit SHA in the URL is stronger, and verifying the
  attestation of a release-asset `run.sh` before running it is stronger still.
  Tags can be moved by someone with repository write access, so use a commit
  SHA when the exact script matters.

## Scope

- **Path traversal and symlinks.** Skill names must be one plain path
  component; absolute names, `/`, `\`, `.`, `..`, and control characters are
  rejected for `skills link` and `skills unlink`, and for discovered skill
  directories. The tool only creates symlinks and removes symlinks it manages;
  it never overwrites an existing file, directory, or unrelated symlink. Each
  destination is re-checked immediately before it is changed. That check is not
  atomic with the change, so a concurrent replacement in the gap is not
  prevented.
- **Terminal output.** Skill directory names containing control characters are
  rejected at discovery, so they cannot inject escape sequences into the
  plain-text tables, the TUI, or error messages.
- **Downloads and cache.** Downloads are HTTPS-only (`--proto '=https'` with
  redirects restricted to `https`), come from this repository's release URLs,
  and are verified against the SHA-256 checksum before the binary is executed.
  A mismatch aborts the run.
- **Platforms.** macOS and Linux on x64 and arm64 are supported; the static
  Linux binaries also run on musl systems. Native Windows is not supported, and
  WSL uses the Linux assets.
