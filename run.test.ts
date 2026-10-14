import { describe, expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";

const RUN_SH = join(import.meta.dir, "run.sh");
const REPOSITORY = "johnnymayodev/Global-AGENTS.md";
const VERSION = "0.1.0";

function sha256(contents: string | Buffer): string {
  return createHash("sha256").update(contents).digest("hex");
}

function writeExecutable(path: string, contents: string): void {
  writeFileSync(path, contents);
  chmodSync(path, 0o755);
}

type RunnerFixture = {
  tempDir: string;
  binDir: string;
  homeDir: string;
  curlUrls: string;
  args: string;
  pwd: string;
  assetExe: string;
  assetSource: string;
  env: NodeJS.ProcessEnv;
};

const CURL_STUB = `#!/bin/sh
set -eu
output=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      output=$2
      shift 2
      ;;
    https://*)
      url=$1
      shift
      ;;
    *)
      shift
      ;;
  esac
done
printf '%s\\n' "$url" >> "$CURL_URL_CAPTURE"
if [ -n "\${CURL_FAIL:-}" ]; then
  exit "\${CURL_FAIL:-22}"
fi
if [ -n "\${CURL_PARTIAL:-}" ]; then
  printf 'partial' > "$output"
  exit 18
fi
base=$(basename "$output")
case "$base" in
  checksums-*.txt)
    # The test pre-writes the checksum file next to the fixture; prefer an
    # explicit override, else fall back to the file the test already staged.
    if [ -n "\${CURL_CHECKSUM_FILE:-}" ]; then
      cp "$CURL_CHECKSUM_FILE" "$output"
    else
      cp "$TMPDIR/$base" "$output"
    fi
    ;;
  *)
    cp "$CURL_SOURCE_FILE" "$output"
    ;;
esac
`;

const ASSET_TEMPLATE = `#!/bin/sh
set -eu
printf '%s\\n' "$0" > "$ASSET_EXE_CAPTURE"
printf '%s\\n' "$PWD" > "$PWD_CAPTURE"
: > "$ARGS_CAPTURE"
for arg in "$@"; do
  printf '%s\\n' "$arg" >> "$ARGS_CAPTURE"
done
if [ -n "\${ASSET_STDIN_TTY:-}" ]; then
  if [ -t 0 ]; then
    printf 'tty' > "$ASSET_STDIN_TTY"
  else
    printf 'not-tty' > "$ASSET_STDIN_TTY"
  fi
fi
exit "\${ASSET_EXIT:-0}"
`;

const UNAME_TEMPLATE = `#!/bin/sh
set -eu
case "$1" in
  -s) printf '%s\\n' "$UNAME_OS" ;;
  -m) printf '%s\\n' "$UNAME_MACHINE" ;;
  *) exit 1 ;;
esac
`;

function baseFixture(): RunnerFixture {
  const tempDir = mkdtempSync(join(tmpdir(), "agent-links-runner-"));
  const binDir = join(tempDir, "bin");
  const homeDir = join(tempDir, "home");
  mkdirSync(binDir);
  mkdirSync(homeDir);
  const fixture: RunnerFixture = {
    tempDir,
    binDir,
    homeDir,
    curlUrls: join(tempDir, "curl-urls"),
    args: join(tempDir, "asset-args"),
    pwd: join(tempDir, "asset-pwd"),
    assetExe: join(tempDir, "asset-exe"),
    assetSource: join(tempDir, "asset-source"),
    env: {},
  };
  writeExecutable(join(binDir, "curl"), CURL_STUB);
  writeExecutable(join(binDir, "uname"), UNAME_TEMPLATE);
  writeFileSync(fixture.assetSource, ASSET_TEMPLATE);
  fixture.env = {
    ...process.env,
    PATH: `${binDir}${delimiter}/usr/bin${delimiter}/bin`,
    TMPDIR: tempDir,
    CURL_URL_CAPTURE: fixture.curlUrls,
    CURL_SOURCE_FILE: fixture.assetSource,
    ASSET_EXE_CAPTURE: fixture.assetExe,
    ARGS_CAPTURE: fixture.args,
    PWD_CAPTURE: fixture.pwd,
    HOME: homeDir,
    UNAME_OS: process.platform === "darwin" ? "Darwin" : "Linux",
    UNAME_MACHINE: process.arch === "arm64" ? "aarch64" : "x86_64",
  };
  return fixture;
}

function withChecksum(fixture: RunnerFixture, assetName: string): void {
  const target = assetName.replace(/^agent-links-/, "");
  const contents = `${sha256(readFileSync(fixture.assetSource))}  ${assetName}\n`;
  const path = join(fixture.tempDir, `checksums-${target}.txt`);
  writeFileSync(path, contents);
  fixture.env.CURL_CHECKSUM_FILE = path;
}

function runFixture(
  fixture: RunnerFixture,
  args: string[],
  overrides: NodeJS.ProcessEnv = {},
  cwd?: string,
) {
  return spawnSync("bash", [RUN_SH, ...args], {
    encoding: "utf8",
    env: { ...fixture.env, ...overrides },
    cwd: cwd ?? fixture.tempDir,
  });
}

function fakeUname(fixture: RunnerFixture, os: string, machine: string): void {
  writeExecutable(join(fixture.binDir, "uname"), UNAME_TEMPLATE);
  fixture.env.UNAME_OS = os;
  fixture.env.UNAME_MACHINE = machine;
}

function fakeMusl(fixture: RunnerFixture): void {
  writeExecutable(
    join(fixture.binDir, "ls"),
    `#!/bin/sh
set -eu
if [ "$1" = '/lib/ld-musl-*' ] && [ "\${FAKE_MUSL:-0}" = 1 ]; then
  exit 0
fi
exit 1
`,
  );
}

function assertClean(fixture: RunnerFixture): void {
  const leftovers = readdirSync(fixture.tempDir).filter((entry) =>
    entry.startsWith("agent-links."),
  );
  expect(leftovers).toEqual([]);
}

function hostTarget(): string {
  const os = process.platform === "darwin" ? "darwin" : "linux";
  const arch = process.arch === "arm64" ? "arm64" : "x64";
  return `${os}-${arch}`;
}

function releaseUrl(target: string): string {
  return `https://github.com/${REPOSITORY}/releases/download/v${VERSION}/agent-links-${target}`;
}

function checksumUrl(target: string): string {
  return `https://github.com/${REPOSITORY}/releases/download/v${VERSION}/checksums-${target}.txt`;
}

function hasHashTool(): boolean {
  return (
    spawnSync("sh", ["-c", "command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1"])
      .status === 0
  );
}

const HASH_TOOL_OK = hasHashTool();
const PTY_AVAILABLE =
  spawnSync("sh", ["-c", "command -v script >/dev/null 2>&1"]).status === 0;

describe("platform detection", () => {
  const cases: Array<[string, string, string]> = [
    ["Darwin", "x86_64", "darwin-x64"],
    ["Darwin", "amd64", "darwin-x64"],
    ["Darwin", "aarch64", "darwin-arm64"],
    ["Darwin", "arm64", "darwin-arm64"],
    ["Linux", "x86_64", "linux-x64"],
    ["Linux", "amd64", "linux-x64"],
    ["Linux", "aarch64", "linux-arm64"],
  ];

  for (const [os, machine, target] of cases) {
    test(`maps ${os} ${machine} to ${target}`, () => {
      if (!HASH_TOOL_OK) return;
      const fixture = baseFixture();
      try {
        fakeUname(fixture, os, machine);
        withChecksum(fixture, `agent-links-${target}`);

        const result = runFixture(fixture, []);

        expect(result.status).toBe(0);
        const urls = readFileSync(fixture.curlUrls, "utf8").trim().split("\n");
        expect(urls[0]).toBe(releaseUrl(target));
        expect(urls[1]).toBe(checksumUrl(target));
        expect(existsSync(fixture.assetExe)).toBe(true);
        assertClean(fixture);
      } finally {
        rmSync(fixture.tempDir, { recursive: true, force: true });
      }
    });
  }

  test("rejects unsupported OS", () => {
    const fixture = baseFixture();
    try {
      fakeUname(fixture, "FreeBSD", "x86_64");
      const result = runFixture(fixture, []);
      expect(result.status).toBe(2);
      expect(result.stderr).toContain("unsupported platform");
      expect(existsSync(fixture.assetExe)).toBe(false);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("rejects unsupported architecture", () => {
    const fixture = baseFixture();
    try {
      fakeUname(fixture, "Linux", "riscv64");
      const result = runFixture(fixture, []);
      expect(result.status).toBe(2);
      expect(result.stderr).toContain("unsupported platform");
      expect(existsSync(fixture.assetExe)).toBe(false);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("rejects musl-based Linux", () => {
    const fixture = baseFixture();
    try {
      fakeUname(fixture, "Linux", "x86_64");
      fakeMusl(fixture);
      fixture.env.FAKE_MUSL = "1";
      const result = runFixture(fixture, []);

      expect(result.status).toBe(2);
      expect(result.stderr).toContain("musl-based Linux");
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });
});

describe("argument forwarding and working directory", () => {
  test("forwards CLI arguments verbatim", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      withChecksum(fixture, `agent-links-${hostTarget()}`);
      const result = runFixture(fixture, ["--path", "my config/AGENTS.md", "./relative"]);

      expect(result.status).toBe(0);
      const args = readFileSync(fixture.args, "utf8").trim().split("\n");
      expect(args).toEqual(["--path", "my config/AGENTS.md", "./relative"]);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("preserves a working directory containing spaces and unrelated config files", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      const workDir = join(fixture.tempDir, "some work dir");
      mkdirSync(workDir);
      writeFileSync(join(workDir, ".env"), "SECRET=leaked\n");
      writeFileSync(join(workDir, "bunfig.toml"), "[install]\nregistry = 'https://example.com'\n");
      withChecksum(fixture, `agent-links-${hostTarget()}`);

      const result = runFixture(fixture, [], {}, workDir);

      expect(result.status).toBe(0);
      // macOS /var is a symlink to /private/var; compare real paths.
      expect(realpathSync(readFileSync(fixture.pwd, "utf8").trim())).toBe(
        realpathSync(workDir),
      );
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("works without a JavaScript runtime on PATH", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      writeFileSync(join(fixture.binDir, "bun"), "");
      chmodSync(join(fixture.binDir, "bun"), 0o644);
      writeFileSync(join(fixture.binDir, "node"), "");
      chmodSync(join(fixture.binDir, "node"), 0o644);
      withChecksum(fixture, `agent-links-${hostTarget()}`);

      const result = runFixture(fixture, []);

      expect(result.status).toBe(0);
      expect(existsSync(fixture.assetExe)).toBe(true);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });
});

describe("failure modes", () => {
  test("checksum mismatch never executes the asset", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      writeFileSync(
        join(fixture.tempDir, `checksums-${hostTarget()}.txt`),
        `${"0".repeat(64)}  agent-links-${hostTarget()}\n`,
      );

      const result = runFixture(fixture, []);

      expect(result.status).toBe(1);
      expect(result.stderr).toContain("checksum verification failed");
      expect(existsSync(fixture.assetExe)).toBe(false);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("missing checksum entry is rejected", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      writeFileSync(join(fixture.tempDir, `checksums-${hostTarget()}.txt`), "");

      const result = runFixture(fixture, []);

      expect(result.status).toBe(1);
      expect(result.stderr).toContain("checksum");
      expect(existsSync(fixture.assetExe)).toBe(false);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("download failure is reported and cleans up", () => {
    const fixture = baseFixture();
    try {
      const result = runFixture(fixture, [], { CURL_FAIL: "1" });

      expect(result.status).toBe(1);
      expect(result.stderr).toContain("failed to download");
      expect(existsSync(fixture.assetExe)).toBe(false);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("partial download is not executed", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      const result = runFixture(fixture, [], { CURL_PARTIAL: "1" });

      expect(result.status).toBe(1);
      expect(result.stderr).toContain("failed to download");
      expect(existsSync(fixture.assetExe)).toBe(false);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("propagates executable failure status", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      withChecksum(fixture, `agent-links-${hostTarget()}`);

      const result = runFixture(fixture, [], { ASSET_EXIT: "7" });

      expect(result.status).toBe(7);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("cleans up when the executable crashes", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      writeFileSync(fixture.assetSource, "#!/bin/sh\nexit 99\n");
      withChecksum(fixture, `agent-links-${hostTarget()}`);

      const result = runFixture(fixture, []);

      expect(result.status).toBe(99);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });

  test("reports a launch problem and cleans up for an invalid executable", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      writeFileSync(fixture.assetSource, "#!/does-not-exist/agent-links\n");
      withChecksum(fixture, `agent-links-${hostTarget()}`);

      const result = runFixture(fixture, []);

      // Shells differ on the status for a missing shebang interpreter: bash on
      // macOS reports 126, while Linux shells commonly report 127.
      expect([126, 127] as Array<number | null>).toContain(result.status);
      expect(result.stderr).toContain("could not launch");
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });
});

describe("standalone runner entry", () => {
  test("pipelines run.sh through a shell with no arguments", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      withChecksum(fixture, `agent-links-${hostTarget()}`);
      const script = readFileSync(RUN_SH, "utf8");

      const result = spawnSync("bash", ["-s", "--"], {
        encoding: "utf8",
        input: script,
        env: fixture.env,
        cwd: fixture.tempDir,
      });

      expect(result.status).toBe(0);
      expect(existsSync(fixture.assetExe)).toBe(true);
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });
});

describe("interactive input restoration", () => {
  // BSD script takes a command as trailing args; util-linux script takes -c.
  const PTY_SYNTAXES: Array<(cmd: string, transcript: string) => string[]> = [
    (cmd, transcript) => ["-q", transcript, "sh", "-c", cmd],
    (cmd, transcript) => ["-qec", cmd, transcript],
  ];

  test.skipIf(!HASH_TOOL_OK || !PTY_AVAILABLE)(
    "restores interactive input from /dev/tty when stdout is a TTY",
    () => {
      const fixture = baseFixture();
      try {
        withChecksum(fixture, `agent-links-${hostTarget()}`);
        const ttyCapture = join(fixture.tempDir, "asset-stdin-tty");
        const transcript = join(fixture.tempDir, "transcript");
        const cmd = `cat '${RUN_SH}' | bash`;

        let ran = false;
        for (const build of PTY_SYNTAXES) {
          const args = build(cmd, transcript);
          const probe = spawnSync("script", args, {
            encoding: "utf8",
            env: { ...fixture.env, ASSET_STDIN_TTY: ttyCapture },
            cwd: fixture.tempDir,
            timeout: 10000,
          });
          if (probe.error || probe.status !== 0) continue;
          ran = true;
          break;
        }

        if (!ran) {
          // `script` (BSD or util-linux) is unavailable in this environment.
          return;
        }

        expect(readFileSync(ttyCapture, "utf8")).toBe("tty");
        assertClean(fixture);
      } finally {
        rmSync(fixture.tempDir, { recursive: true, force: true });
      }
    },
  );

  test("without a TTY, input is not redirected and no /dev/tty is opened", () => {
    if (!HASH_TOOL_OK) return;
    const fixture = baseFixture();
    try {
      withChecksum(fixture, `agent-links-${hostTarget()}`);
      const ttyCapture = join(fixture.tempDir, "asset-stdin-tty");

      const result = runFixture(fixture, [], { ASSET_STDIN_TTY: ttyCapture });

      expect(result.status).toBe(0);
      expect(readFileSync(ttyCapture, "utf8")).toBe("not-tty");
      assertClean(fixture);
    } finally {
      rmSync(fixture.tempDir, { recursive: true, force: true });
    }
  });
});
