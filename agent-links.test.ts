import { expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readlinkSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
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

test("creates a blank canonical instructions file when selected", () => {
  const home = mkdtempSync(join(tmpdir(), "agent-links-blank-"));
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");

  try {
    const result = runLinkSetup(home, "2\n");

    expect(result.status).toBe(0);
    expect(result.stdout).toContain("No shared instructions file found");
    expect(result.stdout).toContain("Created blank instructions file");
    expect(readFileSync(sourceFile, "utf8")).toBe("");
    expect(readlinkSync(join(home, ".config", "opencode", "AGENTS.md"))).toBe(sourceFile);
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

    const result = runLinkSetup(home, `1\n${selectedFile}\n`);

    expect(result.status).toBe(0);
    expect(result.stdout).toContain(`Created ${sourceFile} from ${selectedFile}`);
    expect(readFileSync(sourceFile, "utf8")).toBe(contents);
    expect(readlinkSync(join(home, ".config", "opencode", "AGENTS.md"))).toBe(sourceFile);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

test("does not prompt or overwrite an existing canonical instructions file", () => {
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
