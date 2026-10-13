#!/usr/bin/env bun

import { createInterface, emitKeypressEvents, type Interface } from "node:readline";
import {
  accessSync,
  constants as fsConstants,
  copyFileSync,
  lstatSync,
  mkdirSync,
  realpathSync,
  readlinkSync,
  statSync,
  symlinkSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import {
  delimiter,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
  sep,
} from "node:path";
import { fileURLToPath } from "node:url";

type Harness = {
  id: string;
  name: string;
  target: string;
  executables: string[];
  installDirs: string[];
};

type Context = {
  home: string;
  sourceFile: string;
  codexHome: string;
  harnesses: Harness[];
};

type TargetState = "managed" | "symlink" | "path" | "absent";

type Keypress = {
  name?: string;
  ctrl?: boolean;
};

type LinkPlan = {
  toCreate: Harness[];
  alreadyLinked: Harness[];
};

type UnlinkPlan = {
  toRemove: Harness[];
  alreadyAbsent: Harness[];
  untouched: Harness[];
};

type ReconcilePlan = LinkPlan & UnlinkPlan;

function absoluteFrom(value: string, base: string): string {
  return isAbsolute(value) ? resolve(value) : resolve(base, value);
}

function buildContext(): Context {
  const home = process.env.HOME;
  if (!home) throw new Error("HOME must be set");
  if (!isAbsolute(home)) throw new Error("HOME must be an absolute path");

  const cwd = process.cwd();
  const sourceFile = join(home, ".config", "agents", "AGENTS.md");
  const codexHome = absoluteFrom(process.env.CODEX_HOME || join(home, ".codex"), cwd);
  const hermesHome = absoluteFrom(process.env.HERMES_HOME || join(home, ".hermes"), cwd);

  const ompConfigSetting = process.env.PI_CONFIG_DIR || ".omp";
  const ompConfigRoot = absoluteFrom(ompConfigSetting, home);
  const ompProfile = Object.hasOwn(process.env, "OMP_PROFILE")
    ? process.env.OMP_PROFILE || ""
    : process.env.PI_PROFILE || "";

  let ompAgentDir: string;
  if (ompProfile) {
    ompAgentDir = join(ompConfigRoot, "profiles", ompProfile, "agent");
  } else if (process.env.PI_CODING_AGENT_DIR) {
    ompAgentDir = absoluteFrom(process.env.PI_CODING_AGENT_DIR, cwd);
  } else {
    ompAgentDir = join(ompConfigRoot, "agent");
  }

  const harnesses: Harness[] = [
    {
      id: "opencode",
      name: "OpenCode",
      target: join(home, ".config", "opencode", "AGENTS.md"),
      executables: ["opencode"],
      installDirs: [join(home, ".config", "opencode")],
    },
    {
      id: "codex",
      name: "Codex",
      target: join(codexHome, "AGENTS.md"),
      executables: ["codex"],
      installDirs: [codexHome],
    },
    {
      id: "pi",
      name: "Pi",
      target: join(home, ".pi", "agent", "AGENTS.md"),
      executables: ["pi"],
      installDirs: [join(home, ".pi", "agent")],
    },
    {
      id: "omp",
      name: "OMP",
      target: join(ompAgentDir, "AGENTS.md"),
      executables: ["omp", "oh-my-pi"],
      installDirs: [ompAgentDir, ompConfigRoot],
    },
    {
      id: "hermes",
      name: "Hermes Agent",
      target: join(hermesHome, "AGENTS.md"),
      executables: ["hermes"],
      installDirs: [hermesHome],
    },
    {
      id: "claude",
      name: "Claude Code",
      target: join(home, ".claude", "CLAUDE.md"),
      executables: ["claude"],
      installDirs: [join(home, ".claude")],
    },
  ];

  return { home, sourceFile, codexHome, harnesses };
}

function missingPathError(error: unknown): boolean {
  if (typeof error !== "object" || error === null || !("code" in error)) return false;
  return error.code === "ENOENT" || error.code === "ENOTDIR";
}

function sourceFileExists(context: Context): boolean {
  let linkStats;
  try {
    linkStats = lstatSync(context.sourceFile);
  } catch (error) {
    if (missingPathError(error)) return false;
    throw error;
  }

  if (linkStats.isSymbolicLink()) {
    let targetStats;
    try {
      targetStats = statSync(context.sourceFile);
    } catch (error) {
      if (missingPathError(error)) {
        throw new Error(`canonical instructions path is a broken symlink: ${context.sourceFile}; no changes made`);
      }
      throw error;
    }
    if (!targetStats.isFile()) {
      throw new Error(`canonical instructions path is not a regular file: ${context.sourceFile}; no changes made`);
    }
    return true;
  }

  if (!linkStats.isFile()) {
    throw new Error(`canonical instructions path is not a regular file: ${context.sourceFile}; no changes made`);
  }
  return true;
}

type LinePrompt = {
  ask: (message: string) => Promise<string | null>;
  close: () => void;
};

function createLinePrompt(): LinePrompt {
  const readline = createInterface({ input: process.stdin, output: process.stdout });
  const answers: string[] = [];
  const waiters: Array<(answer: string | null) => void> = [];
  let closed = false;

  readline.on("line", (answer) => {
    const waiter = waiters.shift();
    if (waiter) waiter(answer);
    else answers.push(answer);
  });
  readline.on("close", () => {
    closed = true;
    for (const waiter of waiters.splice(0)) waiter(null);
  });
  readline.on("SIGINT", () => {
    closed = true;
    for (const waiter of waiters.splice(0)) waiter(null);
    readline.close();
  });

  return {
    ask(message) {
      process.stdout.write(message);
      const answer = answers.shift();
      if (answer !== undefined) return Promise.resolve(answer);
      if (closed) return Promise.resolve(null);
      return new Promise((resolveAnswer) => waiters.push(resolveAnswer));
    },
    close() {
      readline.close();
      process.stdin.pause();
    },
  };
}

function selectedFilePath(value: string, context: Context): string {
  const trimmed = value.trim();
  const expanded = trimmed === "~"
    ? context.home
    : trimmed.startsWith("~/") || trimmed.startsWith(`~${sep}`)
      ? join(context.home, trimmed.slice(2))
      : trimmed;
  return absoluteFrom(expanded, process.cwd());
}

async function ensureSourceFile(context: Context): Promise<boolean> {
  if (sourceFileExists(context)) return true;

  console.log(`No shared instructions file found at ${context.sourceFile}.`);
  const prompt = createLinePrompt();
  try {
    while (true) {
      const choice = await prompt.ask(
        "Choose: [1] copy an existing instructions file  [2] create a blank AGENTS.md  [q] cancel: ",
      );
      if (choice === null || choice.trim().toLowerCase() === "q") {
        console.log("Cancelled. No changes made.");
        return false;
      }

      switch (choice.trim().toLowerCase()) {
        case "1": {
          const path = await prompt.ask("Path to the existing instructions file: ");
          if (path === null) {
            console.log("Cancelled. No changes made.");
            return false;
          }

          const selectedPath = selectedFilePath(path, context);
          let selectedStats;
          try {
            selectedStats = statSync(selectedPath);
          } catch (error) {
            console.error(
              missingPathError(error)
                ? `not found: ${selectedPath}`
                : `cannot read: ${selectedPath}`,
            );
            continue;
          }
          if (!selectedStats.isFile()) {
            console.error(`not a regular file: ${selectedPath}`);
            continue;
          }

          mkdirSync(dirname(context.sourceFile), { recursive: true });
          copyFileSync(selectedPath, context.sourceFile, fsConstants.COPYFILE_EXCL);
          console.log(`Created ${context.sourceFile} from ${selectedPath}`);
          return true;
        }
        case "2":
          mkdirSync(dirname(context.sourceFile), { recursive: true });
          writeFileSync(context.sourceFile, "", { flag: "wx" });
          console.log(`Created blank instructions file: ${context.sourceFile}`);
          return true;
        default:
          console.log('Enter "1" to copy a file, "2" to create a blank file, or "q" to cancel.');
      }
    }
  } finally {
    prompt.close();
  }
}

function targetState(target: string, sourceFile: string): TargetState {
  let stats;
  try {
    stats = lstatSync(target);
  } catch (error) {
    if (missingPathError(error)) return "absent";
    throw error;
  }

  if (stats.isSymbolicLink()) {
    return readlinkSync(target) === sourceFile ? "managed" : "symlink";
  }

  return "path";
}

function hasExecutable(name: string): boolean {
  const searchPath = process.env.PATH || "";
  for (const entry of searchPath.split(delimiter)) {
    const candidate = join(entry || process.cwd(), name);
    try {
      accessSync(candidate, fsConstants.X_OK);
      if (statSync(candidate).isFile()) return true;
    } catch {
      // Keep searching PATH.
    }
  }
  return false;
}

function isDirectory(path: string): boolean {
  try {
    return statSync(path).isDirectory();
  } catch (error) {
    if (missingPathError(error)) return false;
    throw error;
  }
}

function isInstalled(harness: Harness, context: Context): boolean {
  if (targetState(harness.target, context.sourceFile) === "managed") return true;
  if (harness.executables.some(hasExecutable)) return true;
  return harness.installDirs.some(isDirectory);
}

function displayPath(path: string, home: string): string {
  const fromHome = relative(home, path);
  if (!fromHome || fromHome === ".") return "~";
  if (fromHome !== ".." && !fromHome.startsWith(`..${sep}`)) return `~/${fromHome}`;
  return path;
}

function renderTable(
  harnesses: Harness[],
  context: Context,
  selected: Set<string>,
  focusedIndex = -1,
): string {
  const headers = ["", "Harness", "Installed", "Global instructions"];
  const rows = harnesses.map((harness, index) => [
    focusedIndex === index ? ">" : "",
    `[${selected.has(harness.id) ? "x" : " "}] ${harness.name}`,
    isInstalled(harness, context) ? "Yes" : "No",
    displayPath(harness.target, context.home),
  ]);

  if (rows.length === 0) {
    rows.push(["", "(none detected)", "", ""]);
  }

  const widths = headers.map((header, index) =>
    Math.max(header.length, ...rows.map((row) => row[index]?.length ?? 0)),
  );
  const border = (left: string, middle: string, right: string) =>
    left + widths.map((width) => "─".repeat(width + 2)).join(middle) + right;
  const row = (cells: string[]) =>
    `│ ${cells.map((cell, index) => cell.padEnd(widths[index] ?? cell.length)).join(" │ ")} │`;

  const lines = [
    `Global instructions: ${context.sourceFile}`,
    "",
    border("┌", "┬", "┐"),
    row(headers),
    border("├", "┼", "┤"),
    ...rows.map(row),
    border("└", "┴", "┘"),
  ];

  return lines.join("\n");
}

function assertSourceFile(context: Context): void {
  if (sourceFileExists(context)) return;
  throw new Error(`missing canonical instructions file: ${context.sourceFile}; no changes made`);
}

function planLinks(targets: Harness[], context: Context): LinkPlan {
  const plan: LinkPlan = { toCreate: [], alreadyLinked: [] };
  if (targets.length === 0) return plan;
  assertSourceFile(context);

  const seen = new Set<string>();
  for (const harness of targets) {
    if (seen.has(harness.target)) continue;
    seen.add(harness.target);

    const state = targetState(harness.target, context.sourceFile);
    if (state === "managed") {
      plan.alreadyLinked.push(harness);
    } else if (state === "absent") {
      plan.toCreate.push(harness);
    } else if (state === "symlink") {
      throw new Error(`refusing to replace unrelated symlink: ${harness.target}; no changes made`);
    } else {
      throw new Error(`refusing to replace existing path: ${harness.target}; no changes made`);
    }
  }

  return plan;
}

function planUnlinks(targets: Harness[], context: Context): UnlinkPlan {
  const plan: UnlinkPlan = { toRemove: [], alreadyAbsent: [], untouched: [] };
  const seen = new Set<string>();

  for (const harness of targets) {
    if (seen.has(harness.target)) continue;
    seen.add(harness.target);

    const state = targetState(harness.target, context.sourceFile);
    if (state === "managed") plan.toRemove.push(harness);
    else if (state === "absent") plan.alreadyAbsent.push(harness);
    else plan.untouched.push(harness);
  }

  return plan;
}

function printLinkPlan(plan: LinkPlan, context: Context, heading = "Link plan"): void {
  console.log(`${heading}:`);
  if (plan.toCreate.length > 0) {
    console.log("  Create these symlinks (creating missing parent directories if needed):");
    for (const harness of plan.toCreate) {
      console.log(`    ${harness.target} -> ${context.sourceFile}`);
    }
  }
  if (plan.alreadyLinked.length > 0) {
    console.log("  Already linked:");
    for (const harness of plan.alreadyLinked) {
      console.log(`    ${harness.target} -> ${context.sourceFile}`);
    }
  }
  if (plan.toCreate.length === 0) console.log("  No symlinks need to be created.");
  console.log("  Existing files and unrelated symlinks will not be overwritten.");
}

function printUnlinkPlan(plan: UnlinkPlan, context: Context, heading = "Unlink plan"): void {
  console.log(`${heading}:`);
  if (plan.toRemove.length > 0) {
    console.log("  Remove only these managed symlink entries (not their target files):");
    for (const harness of plan.toRemove) {
      console.log(`    ${harness.target} -> ${context.sourceFile}`);
    }
  }
  for (const harness of plan.alreadyAbsent) {
    console.log(`  Already absent: ${harness.target}`);
  }
  for (const harness of plan.untouched) {
    console.log(`  Leave untouched (not a managed symlink): ${harness.target}`);
  }
  if (plan.toRemove.length === 0) console.log("  No managed symlinks will be removed.");
  console.log("  Regular files and unrelated symlinks will be left untouched.");
}

function printReconcilePlan(plan: ReconcilePlan, context: Context): void {
  console.log("Apply plan:");
  if (plan.toCreate.length > 0) {
    console.log("  Create these symlinks (creating missing parent directories if needed):");
    for (const harness of plan.toCreate) {
      console.log(`    ${harness.target} -> ${context.sourceFile}`);
    }
  }
  if (plan.toRemove.length > 0) {
    console.log("  Remove only these managed symlink entries (after confirmation):");
    for (const harness of plan.toRemove) {
      console.log(`    ${harness.target} -> ${context.sourceFile}`);
    }
  }
  if (plan.alreadyLinked.length > 0) {
    console.log("  Keep already-linked destinations:");
    for (const harness of plan.alreadyLinked) {
      console.log(`    ${harness.target} -> ${context.sourceFile}`);
    }
  }
  for (const harness of plan.alreadyAbsent) {
    console.log(`  Already absent: ${harness.target}`);
  }
  for (const harness of plan.untouched) {
    console.log(`  Leave untouched (not a managed symlink): ${harness.target}`);
  }
  if (plan.toCreate.length === 0 && plan.toRemove.length === 0) {
    console.log("  No filesystem changes are planned.");
  }
  console.log("  Existing files and unrelated symlinks will not be overwritten or removed.");
}

function printLinkNotes(targets: Harness[], context: Context): void {
  if (targets.some((harness) => harness.id === "codex")) {
    try {
      const override = statSync(join(context.codexHome, "AGENTS.override.md"));
      if (override.size > 0) {
        console.error(
          `note: Codex prefers its non-empty ${join(context.codexHome, "AGENTS.override.md")} over AGENTS.md`,
        );
      }
    } catch (error) {
      if (!missingPathError(error)) throw error;
    }
  }

  if (targets.some((harness) => harness.id === "hermes")) {
    console.error(
      "note: Hermes global AGENTS.md loading depends on Hermes version;\n" +
        "      current docs guarantee global SOUL.md, not AGENTS.md",
    );
  }
}

function createLinks(targets: Harness[], context: Context): void {
  for (const harness of targets) {
    let directoryCreationStarted = false;
    try {
      const state = targetState(harness.target, context.sourceFile);
      if (state === "managed") {
        console.log(`already linked ${harness.target} -> ${context.sourceFile}`);
        continue;
      }
      if (state !== "absent") {
        throw new Error(`destination changed after planning; refusing to overwrite: ${harness.target}`);
      }

      directoryCreationStarted = true;
      const createdDirectory = mkdirSync(dirname(harness.target), { recursive: true });
      if (createdDirectory) console.log(`created directory ${createdDirectory}`);
      try {
        symlinkSync(context.sourceFile, harness.target);
      } catch (error) {
        if (
          typeof error === "object" &&
          error !== null &&
          "code" in error &&
          error.code === "EEXIST"
        ) {
          throw new Error(`destination appeared after planning; refusing to overwrite: ${harness.target}`);
        }
        throw error;
      }
    } catch (error) {
      console.error(
        `stopped at ${harness.target}; earlier reported changes remain,${directoryCreationStarted ? " directory creation for this destination may be partial," : ""} and later destinations were not attempted.`,
      );
      throw error;
    }
    console.log(`created symlink ${harness.target} -> ${context.sourceFile}`);
  }
}

function linkTargets(targets: Harness[], context: Context): void {
  const plan = planLinks(targets, context);
  printLinkPlan(plan, context);
  if (plan.toCreate.length === 0) console.log("No changes made.");
  else createLinks(plan.toCreate, context);
  printLinkNotes(targets, context);
}

function removeManagedLinks(targets: Harness[], context: Context): void {
  for (const harness of targets) {
    try {
      const state = targetState(harness.target, context.sourceFile);
      if (state === "managed") {
        unlinkSync(harness.target);
        console.log(`removed managed symlink ${harness.target}`);
      } else if (state === "absent") {
        console.log(`left untouched (already absent): ${harness.target}`);
      } else {
        console.log(`left untouched (no longer a managed symlink): ${harness.target}`);
      }
    } catch (error) {
      console.error(
        `stopped at ${harness.target}; earlier reported removals remain, and later destinations were not attempted.`,
      );
      throw error;
    }
  }
}

async function confirmManagedLinkRemoval(targets: Harness[]): Promise<boolean> {
  if (targets.length === 0) return true;
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.error("error: removal requires interactive confirmation; no changes made");
    process.exitCode = 1;
    return false;
  }

  const readline = createInterface({ input: process.stdin, output: process.stdout });
  return new Promise((resolveConfirmation) => {
    let finished = false;
    const finish = (confirmed: boolean) => {
      if (finished) return;
      finished = true;
      readline.close();
      if (confirmed) {
        console.log(`Confirmation received; removing ${targets.length} managed symlink(s).`);
      } else {
        console.log("Cancelled. No changes made.");
      }
      resolveConfirmation(confirmed);
    };

    readline.once("close", () => {
      if (finished) return;
      finished = true;
      console.log("Cancelled (no confirmation received). No changes made.");
      resolveConfirmation(false);
    });
    readline.once("SIGINT", () => finish(false));
    readline.question(
      `Type "yes" to remove the ${targets.length} managed symlink(s) listed above: `,
      (answer) => finish(answer.trim().toLowerCase() === "yes"),
    );
  });
}

async function unlinkTargets(targets: Harness[], context: Context): Promise<void> {
  const plan = planUnlinks(targets, context);
  printUnlinkPlan(plan, context);
  if (plan.toRemove.length === 0) {
    console.log("No changes made.");
    return;
  }
  if (!(await confirmManagedLinkRemoval(plan.toRemove))) return;
  removeManagedLinks(plan.toRemove, context);
}

async function reconcileSelection(
  visibleHarnesses: Harness[],
  selectedIds: Set<string>,
  context: Context,
): Promise<void> {
  const selected = visibleHarnesses.filter((harness) => selectedIds.has(harness.id));
  const selectedTargets = new Set(selected.map((harness) => harness.target));
  const deselected = visibleHarnesses.filter(
    (harness) => !selectedIds.has(harness.id) && !selectedTargets.has(harness.target),
  );

  const plan: ReconcilePlan = {
    ...planLinks(selected, context),
    ...planUnlinks(deselected, context),
  };
  printReconcilePlan(plan, context);
  if (plan.toCreate.length === 0 && plan.toRemove.length === 0) {
    console.log("No changes made.");
    return;
  }
  if (!(await confirmManagedLinkRemoval(plan.toRemove))) return;

  // Create new links first so a failed addition cannot remove an existing link.
  try {
    createLinks(plan.toCreate, context);
  } catch (error) {
    if (plan.toRemove.length > 0) {
      console.error("No planned managed links were removed because link creation did not finish.");
    }
    throw error;
  }
  removeManagedLinks(plan.toRemove, context);
  printLinkNotes(selected, context);
}

function sameSelection(left: Set<string>, right: Set<string>): boolean {
  return left.size === right.size && [...left].every((id) => right.has(id));
}

function getVisibleHarnesses(context: Context, showAll: boolean): Harness[] {
  return showAll
    ? context.harnesses
    : context.harnesses.filter((harness) => isInstalled(harness, context));
}

function printList(context: Context, showAll: boolean): void {
  const harnesses = getVisibleHarnesses(context, showAll);
  const selected = new Set(
    harnesses
      .filter((harness) => targetState(harness.target, context.sourceFile) === "managed")
      .map((harness) => harness.id),
  );

  console.log(renderTable(harnesses, context, selected));
  if (harnesses.length === 0) {
      console.log("No supported harnesses were detected. Use `agent-links --all` to show every supported target.");
  }
}

function chooseHarnesses(harnesses: Harness[], context: Context): Promise<Set<string> | null> {
  const initiallySelected = new Set(
    harnesses
      .filter((harness) => targetState(harness.target, context.sourceFile) === "managed")
      .map((harness) => harness.id),
  );

  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    console.log(renderTable(harnesses, context, initiallySelected));
    console.log("Interactive selection needs a terminal; no changes made.");
    return Promise.resolve(null);
  }

  if (harnesses.length === 0) {
    console.log(renderTable(harnesses, context, initiallySelected));
    console.log("No supported harnesses were detected. Use `agent-links --all` to show every supported target.");
    return Promise.resolve(null);
  }

  return new Promise((resolveSelection, rejectSelection) => {
    let cursor = 0;
    let finished = false;
    const originalRawMode = process.stdin.isRaw;

    const render = () => {
      process.stdout.write("\u001b[2J\u001b[H");
      process.stdout.write(`${renderTable(harnesses, context, initiallySelected, cursor)}\n\n`);
      process.stdout.write(
        "↑/↓ move  Space toggle  A select all  N select none  Enter apply  Esc cancel\n" +
          "Managed links are removed only after showing a plan and receiving confirmation.",
      );
    };

    const cleanup = () => {
      process.stdin.off("keypress", onKeypress as never);
      if (process.stdin.isTTY) process.stdin.setRawMode(originalRawMode || false);
      process.stdin.pause();
      process.stdout.write("\u001b[?25h\u001b[0m\n");
    };

    const finish = (selection: Set<string> | null) => {
      if (finished) return;
      finished = true;
      cleanup();
      resolveSelection(selection);
    };

    const onKeypress = (input: string, key?: Keypress) => {
      if (key?.ctrl && key.name === "c") {
        finish(null);
        return;
      }

      switch (key?.name) {
        case "up":
          cursor = (cursor + harnesses.length - 1) % harnesses.length;
          render();
          return;
        case "down":
          cursor = (cursor + 1) % harnesses.length;
          render();
          return;
        case "return":
          finish(new Set(initiallySelected));
          return;
        case "escape":
          finish(null);
          return;
      }

      if (input === " ") {
        const harness = harnesses[cursor];
        if (!harness) return;
        const id = harness.id;
        if (initiallySelected.has(id)) initiallySelected.delete(id);
        else initiallySelected.add(id);
        render();
      } else if (input.toLowerCase() === "a") {
        for (const harness of harnesses) initiallySelected.add(harness.id);
        render();
      } else if (input.toLowerCase() === "n") {
        initiallySelected.clear();
        render();
      } else if (input === "\u0003") {
        finish(null);
      }
    };

    try {
      emitKeypressEvents(process.stdin);
      process.stdin.setRawMode(true);
      process.stdin.resume();
      process.stdin.on("keypress", onKeypress as never);
      process.stdout.write("\u001b[?25l");
      render();
    } catch (error) {
      finished = true;
      try {
        cleanup();
      } catch {
        // Preserve the original setup error.
      }
      rejectSelection(error);
    }
  });
}

function printUsage(): void {
  console.log(
    [
      "usage:",
      "  agent-links                 choose detected harnesses interactively",
      "  agent-links --all           choose from all supported harnesses",
      "  agent-links --list [--all]  print the detected (or all) harnesses",
      "  agent-links link            preview and link all supported harnesses",
      "  agent-links unlink          preview and confirm removal of managed links",
    ].join("\n"),
  );
}

export async function runAgentLinks(args: string[] = process.argv.slice(2)): Promise<void> {
  if (args.length === 1 && (args[0] === "--help" || args[0] === "-h")) {
    printUsage();
    return;
  }

  const context = buildContext();

  if (args.length === 0 || (args.length === 1 && args[0] === "--all")) {
    const showAll = args[0] === "--all";
    if (!(await ensureSourceFile(context))) return;
    const visibleHarnesses = getVisibleHarnesses(context, showAll);
    const currentSelection = new Set(
      visibleHarnesses
        .filter((harness) => targetState(harness.target, context.sourceFile) === "managed")
        .map((harness) => harness.id),
    );
    const selected = await chooseHarnesses(visibleHarnesses, context);
    if (selected === null) {
      console.log("Cancelled. No changes made.");
      return;
    }
    if (sameSelection(currentSelection, selected)) {
      console.log("No changes made.");
      return;
    }

    await reconcileSelection(visibleHarnesses, selected, context);
    return;
  }

  if (args[0] === "--list" && (args.length === 1 || (args.length === 2 && args[1] === "--all"))) {
    printList(context, args[1] === "--all");
    return;
  }

  if (args.length === 1 && args[0] === "link") {
    if (!(await ensureSourceFile(context))) return;
    linkTargets(context.harnesses, context);
    return;
  }

  if (args.length === 1 && args[0] === "unlink") {
    await unlinkTargets(context.harnesses, context);
    return;
  }

  printUsage();
  process.exitCode = 2;
}

if (
  process.argv[1] &&
  realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))
) {
  runAgentLinks().catch((error: unknown) => {
    const message = error instanceof Error ? error.message : String(error);
    console.error(`error: ${message}`);
    process.exitCode = 1;
  });
}
