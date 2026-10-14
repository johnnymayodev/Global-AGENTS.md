import { describe, expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { tmpdir } from "node:os";
import { join } from "node:path";

const ROOT = import.meta.dir;
const BUILD_STANDALONE = join(ROOT, "build-standalone.sh");

function bunAvailable(): boolean {
  return spawnSync("bun", ["--version"]).status === 0;
}

const BUN_OK = bunAvailable();
const HOST_TARGET = `bun-${process.platform}-${process.arch}`;
const PYTHON_AVAILABLE = spawnSync("python3", ["--version"]).status === 0;

function buildStandalone(target: string, outfile: string) {
  return spawnSync("sh", [BUILD_STANDALONE, target, outfile], {
    encoding: "utf8",
    cwd: ROOT,
  });
}

describe("build-standalone.sh", () => {
  test("rejects a malformed target", () => {
    const result = spawnSync("sh", [BUILD_STANDALONE, "windows-x64"], {
      encoding: "utf8",
      cwd: ROOT,
    });
    expect(result.status).toBe(2);
    expect(result.stderr).toContain("unsupported target");
  });

  test.skipIf(!BUN_OK)("builds a working host binary that ignores cwd .env and bunfig.toml", () => {
    const tempDir = mkdtempSync(join(tmpdir(), "agent-links-standalone-"));
    try {
      const outfile = join(tempDir, "agent-links-test");
      const build = buildStandalone(HOST_TARGET, outfile);
      expect(build.status).toBe(0);
      expect(existsSync(outfile)).toBe(true);

      // A hostile .env and bunfig.toml in the working directory must not leak
      // into the compiled executable (autoload flags are baked in).
      const cwd = join(tempDir, "cwd with spaces");
      mkdirSync(cwd);
      writeFileSync(join(cwd, ".env"), "CODEX_HOME=leaked-config\n");
      writeFileSync(join(cwd, "bunfig.toml"), "[install]\nregistry = 'https://evil.example'\n");

      const result = spawnSync(outfile, ["--list", "--all"], {
        encoding: "utf8",
        cwd,
        env: {
          ...process.env,
          HOME: tempDir,
          CODEX_HOME: undefined,
          PATH: "/usr/bin:/bin",
        },
      });

      expect(result.status).toBe(0);
      expect(result.stdout).toContain("Harness");
      expect(result.stdout).toContain("~/.codex/AGENTS.md");
      expect(result.stdout).not.toContain("leaked-config");
      expect(result.stderr ?? "").toBe("");
    } finally {
      rmSync(tempDir, { recursive: true, force: true });
    }
  });

  test.skipIf(!BUN_OK)("passes CLI arguments through to the compiled binary", () => {
    const tempDir = mkdtempSync(join(tmpdir(), "agent-links-standalone-args-"));
    try {
      const outfile = join(tempDir, "agent-links-test");
      const build = buildStandalone(HOST_TARGET, outfile);
      expect(build.status).toBe(0);

      const result = spawnSync(outfile, ["--not-a-real-flag"], {
        encoding: "utf8",
        cwd: tempDir,
        env: { ...process.env, HOME: tempDir },
      });

      // Unknown argument prints usage and exits 2 (same as the source entry).
      expect(result.status).toBe(2);
      expect(result.stdout).toContain("usage:");
    } finally {
      rmSync(tempDir, { recursive: true, force: true });
    }
  });

  test.skipIf(!BUN_OK || !PYTHON_AVAILABLE)("keeps the interactive picker, review, and confirmation", () => {
    const tempDir = mkdtempSync(join(tmpdir(), "agent-links-standalone-picker-"));
    try {
      const outfile = join(tempDir, "agent-links-test");
      const build = buildStandalone(HOST_TARGET, outfile);
      expect(build.status).toBe(0);

      const sourceFile = join(tempDir, ".config", "agents", "AGENTS.md");
      mkdirSync(dirname(sourceFile), { recursive: true });
      writeFileSync(sourceFile, "");

      const result = spawnSync(
        "python3",
        ["-c", String.raw`
import os, pty, select, signal, sys, time

pid, master = pty.fork()
if pid == 0:
    os.execv(sys.argv[1], [sys.argv[1], "--all"])

output = bytearray()
sent_selection = False
sent_decline = False
deadline = time.monotonic() + 10
status = None
while time.monotonic() < deadline:
    ready, _, _ = select.select([master], [], [], 0.1)
    if ready:
        try:
            chunk = os.read(master, 4096)
        except OSError:
            chunk = b""
        output.extend(chunk)
        if not sent_selection and b"Space toggle" in output:
            os.write(master, b" \r")
            sent_selection = True
        if sent_selection and not sent_decline and b'Type "yes" to apply the plan' in output:
            os.write(master, b"no\r")
            sent_decline = True
    child, child_status = os.waitpid(pid, os.WNOHANG)
    if child:
        status = child_status
        break

if status is None:
    os.kill(pid, signal.SIGKILL)
    _, status = os.waitpid(pid, 0)
sys.stdout.buffer.write(output)
sys.exit(os.waitstatus_to_exitcode(status))
`, outfile],
        {
          encoding: "utf8",
          env: {
            ...process.env,
            HOME: tempDir,
            CODEX_HOME: join(tempDir, ".codex"),
            HERMES_HOME: join(tempDir, ".hermes"),
            PI_CONFIG_DIR: join(tempDir, ".omp"),
            PI_CODING_AGENT_DIR: join(tempDir, ".omp", "agent"),
          },
          cwd: tempDir,
          timeout: 15000,
        },
      );

      expect(result.status).toBe(0);
      expect(result.stdout).toContain("Space toggle");
      expect(result.stdout).toContain("Apply plan:");
      expect(result.stdout).toContain('Type "yes" to apply the plan above');
      expect(result.stdout).toContain("Cancelled. No changes made.");
      expect(existsSync(join(tempDir, ".config", "opencode", "AGENTS.md"))).toBe(false);
    } finally {
      rmSync(tempDir, { recursive: true, force: true });
    }
  });
});
