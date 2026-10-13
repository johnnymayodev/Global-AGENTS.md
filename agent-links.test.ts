import { expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join, relative } from "node:path";
import { runAgentLinks } from "./agent-links.ts";

function runLinkSetup(home: string, input: string) {
  return spawnSync("bun", [join(import.meta.dir, "agent-links.ts"), "link"], {
    encoding: "utf8",
    env: {
      ...process.env,
      HOME: home,
      CODEX_HOME: join(home, ".codex"),
      HERMES_HOME: join(home, ".hermes"),
      PI_CONFIG_DIR: join(home, ".omp"),
      OMP_PROFILE: "",
      PI_PROFILE: "",
      PI_CODING_AGENT_DIR: join(home, ".omp", "agent"),
    },
    input,
  });
}

function writeExecutable(path: string, contents: string): void {
  writeFileSync(path, contents);
  chmodSync(path, 0o755);
}

test("exports a programmatic CLI entry point", async () => {
  let output = "";
  const originalLog = console.log;
  console.log = (...args) => {
    output = args.join(" ");
  };

  try {
    await runAgentLinks(["--help"]);
  } finally {
    console.log = originalLog;
  }

  expect(output).toContain("agent-links --all");
});

test("launcher prefers Bun, resolves bin symlinks, and falls back to Node", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "agent-links-launcher-"));
  const launcher = join(import.meta.dir, "agent-links");
  const packageDir = join(tempDir, "package");
  const bothBinDir = join(tempDir, "both");
  const nodeBinDir = join(tempDir, "node-only");
  const bunCapturePath = join(tempDir, "bun-capture");
  const bothNodeCapturePath = join(tempDir, "both-node-capture");
  const fallbackNodeCapturePath = join(tempDir, "fallback-node-capture");

  try {
    mkdirSync(packageDir);
    mkdirSync(bothBinDir);
    mkdirSync(nodeBinDir);
    const packageLauncher = join(packageDir, "agent-links");
    copyFileSync(launcher, packageLauncher);
    chmodSync(packageLauncher, 0o755);
    writeExecutable(
      join(bothBinDir, "bun"),
      '#!/bin/sh\nprintf bun > "$BUN_CAPTURE"\n',
    );
    writeExecutable(
      join(bothBinDir, "node"),
      '#!/bin/sh\nprintf node > "$NODE_CAPTURE"\n',
    );
    writeExecutable(
      join(nodeBinDir, "node"),
      '#!/bin/sh\nprintf node > "$NODE_CAPTURE"\n',
    );
    symlinkSync(relative(bothBinDir, packageLauncher), join(bothBinDir, "agent-links"));
    symlinkSync(relative(nodeBinDir, packageLauncher), join(nodeBinDir, "agent-links"));

    const bunPreferred = spawnSync(join(bothBinDir, "agent-links"), ["--help"], {
      encoding: "utf8",
      env: {
        ...process.env,
        PATH: `${bothBinDir}${delimiter}/usr/bin${delimiter}/bin`,
        BUN_CAPTURE: bunCapturePath,
        NODE_CAPTURE: bothNodeCapturePath,
      },
    });

    expect(bunPreferred.status).toBe(0);
    expect(readFileSync(bunCapturePath, "utf8")).toBe("bun");
    expect(existsSync(bothNodeCapturePath)).toBe(false);

    const nodeFallback = spawnSync(join(nodeBinDir, "agent-links"), ["--help"], {
      encoding: "utf8",
      env: {
        ...process.env,
        PATH: `${nodeBinDir}${delimiter}/usr/bin${delimiter}/bin`,
        NODE_CAPTURE: fallbackNodeCapturePath,
      },
    });

    expect(nodeFallback.status).toBe(0);
    expect(readFileSync(fallbackNodeCapturePath, "utf8")).toBe("node");
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});

test("creates a blank canonical instructions file and waits for link approval", () => {
  const home = mkdtempSync(join(tmpdir(), "agent-links-blank-"));
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");
  const opencodeTarget = join(home, ".config", "opencode", "AGENTS.md");
  const claudeTarget = join(home, ".claude", "CLAUDE.md");

  try {
    const setup = runLinkSetup(home, "2\n");

    expect(setup.status).toBe(0);
    expect(setup.stdout).toContain("No shared instructions file found");
    expect(setup.stdout).toContain("Created blank instructions file");
    expect(existsSync(opencodeTarget)).toBe(false);

    const result = runLinkSetup(home, "yes\n");

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("~/.config/opencode/AGENTS.md -> ~/.config/agents/AGENTS.md");
    expect(result.stdout).toContain("~/.claude/CLAUDE.md -> ~/.config/agents/AGENTS.md");
    expect(result.stdout).toContain('Type "yes" to create the 6 symlink(s) listed above:');
    expect(result.stdout).toContain("Confirmation received; creating 6 symlink(s).");
    expect(result.stdout).toContain("created symlink ~/.config/opencode/AGENTS.md");
    expect(result.stdout).toContain("created symlink ~/.claude/CLAUDE.md");
    expect(result.stdout).not.toContain(`created symlink ${opencodeTarget} ->`);
    expect(result.stdout).not.toContain(`created symlink ${claudeTarget} ->`);
    expect(result.stdout.indexOf("Link plan:")).toBeLessThan(
      result.stdout.indexOf('Type "yes" to create the 6 symlink(s) listed above:'),
    );
    expect(result.stdout.indexOf('Type "yes" to create the 6 symlink(s) listed above:')).toBeLessThan(
      result.stdout.indexOf("created symlink ~/.config/opencode/AGENTS.md"),
    );
    expect(readFileSync(sourceFile, "utf8")).toBe("");
    expect(readlinkSync(opencodeTarget)).toBe(sourceFile);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("copies a selected existing instructions file to the canonical location", () => {
  const home = mkdtempSync(join(tmpdir(), "agent-links-copy-"));
  const selectedFile = join(home, "notes", "AGENTS.md");
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");
  const contents = "# Existing instructions\nUse the shared guidelines.\n";

  try {
    mkdirSync(join(home, "notes"));
    writeFileSync(selectedFile, contents);

    const setup = runLinkSetup(home, `1\n${selectedFile}\n`);

    expect(setup.status).toBe(0);
    expect(setup.stdout).toContain(`Created ${sourceFile} from ${selectedFile}`);
    expect(readFileSync(sourceFile, "utf8")).toBe(contents);

    const result = runLinkSetup(home, "yes\n");

    expect(result.status).toBe(0);
    expect(readlinkSync(join(home, ".config", "opencode", "AGENTS.md"))).toBe(sourceFile);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("does not prompt to create a canonical instructions file that already exists", () => {
  const home = mkdtempSync(join(tmpdir(), "agent-links-existing-"));
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");
  const contents = "# Keep these instructions\n";

  try {
    mkdirSync(join(home, ".config", "agents"), { recursive: true });
    writeFileSync(sourceFile, contents);

    const result = runLinkSetup(home, "");

    expect(result.status).toBe(0);
    expect(result.stdout).not.toContain("No shared instructions file found");
    expect(readFileSync(sourceFile, "utf8")).toBe(contents);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("declining the link plan leaves destinations untouched", () => {
  const home = mkdtempSync(join(tmpdir(), "agent-links-decline-"));
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");
  const opencodeTarget = join(home, ".config", "opencode", "AGENTS.md");
  const claudeTarget = join(home, ".claude", "CLAUDE.md");

  try {
    mkdirSync(join(home, ".config", "agents"), { recursive: true });
    writeFileSync(sourceFile, "# Shared instructions\n");

    const result = runLinkSetup(home, "no\n");

    expect(result.status).toBe(0);
    expect(result.stdout).toContain('Type "yes" to create the 6 symlink(s) listed above:');
    expect(result.stdout).toContain("Cancelled. No changes made.");
    expect(existsSync(opencodeTarget)).toBe(false);
    expect(existsSync(claudeTarget)).toBe(false);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("cancelling missing-file setup makes no changes", () => {
  const home = mkdtempSync(join(tmpdir(), "agent-links-cancel-"));
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");

  try {
    const result = runLinkSetup(home, "q\n");

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("Cancelled. No changes made.");
    expect(existsSync(sourceFile)).toBe(false);
    expect(existsSync(join(home, ".config", "opencode", "AGENTS.md"))).toBe(false);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});
