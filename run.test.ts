import { expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";

type Runtime = "bun" | "node";

function writeExecutable(path: string, contents: string): void {
  writeFileSync(path, contents);
  chmodSync(path, 0o755);
}

function createRunnerFixture(runtimes: Runtime[]) {
  const tempDir = mkdtempSync(join(tmpdir(), "agent-links-runner-test-"));
  const binDir = join(tempDir, "bin");
  const fixturePath = join(tempDir, "source.ts");
  const curlUrlCapturePath = join(tempDir, "curl-url");
  const captures = {
    bunScript: join(tempDir, "bun-script"),
    bunSource: join(tempDir, "bun-source"),
    bunArgs: join(tempDir, "bun-args"),
    nodeScript: join(tempDir, "node-script"),
    nodeSource: join(tempDir, "node-source"),
    nodeArgs: join(tempDir, "node-args"),
  };
  const fixture = 'console.log("fixture source");\n';

  mkdirSync(binDir);
  writeFileSync(fixturePath, fixture);

  writeExecutable(
    join(binDir, "curl"),
    String.raw`#!/bin/sh
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
printf '%s\n' "$url" > "$CURL_URL_CAPTURE"
cp "$CURL_SOURCE_FILE" "$output"
`,
  );

  for (const runtime of runtimes) {
    const prefix = runtime.toUpperCase();
    const runtimeArgs = runtime === "bun"
      ? ['[ "$1" = "run" ]', "shift"]
      : [];
    writeExecutable(
      join(binDir, runtime),
      [
        "#!/bin/sh",
        "set -eu",
        ...runtimeArgs,
        "script=$1",
        "shift",
        `printf '%s\\n' "$script" > "$${prefix}_SCRIPT_CAPTURE"`,
        `cp "$script" "$${prefix}_SOURCE_CAPTURE"`,
        `printf '%s\\n' "$@" > "$${prefix}_ARGS_CAPTURE"`,
        "",
      ].join("\n"),
    );
  }

  return {
    tempDir,
    fixture,
    curlUrlCapturePath,
    captures,
    env: {
      ...process.env,
      PATH: `${binDir}${delimiter}/usr/bin${delimiter}/bin`,
      CURL_SOURCE_FILE: fixturePath,
      CURL_URL_CAPTURE: curlUrlCapturePath,
      BUN_SCRIPT_CAPTURE: captures.bunScript,
      BUN_SOURCE_CAPTURE: captures.bunSource,
      BUN_ARGS_CAPTURE: captures.bunArgs,
      NODE_SCRIPT_CAPTURE: captures.nodeScript,
      NODE_SOURCE_CAPTURE: captures.nodeSource,
      NODE_ARGS_CAPTURE: captures.nodeArgs,
    },
  };
}

function runGitHubRunner(env: NodeJS.ProcessEnv) {
  return spawnSync(
    "sh",
    [join(import.meta.dir, "run.sh"), "acme/agent-links", "--help"],
    { encoding: "utf8", env },
  );
}

test("GitHub runner prefers Bun and forwards CLI arguments", () => {
  const fixture = createRunnerFixture(["bun", "node"]);
  try {
    const result = runGitHubRunner(fixture.env);

    expect(result.status).toBe(0);
    expect(readFileSync(fixture.curlUrlCapturePath, "utf8").trim()).toBe(
      "https://raw.githubusercontent.com/acme/agent-links/main/agent-links.ts",
    );
    expect(readFileSync(fixture.captures.bunSource, "utf8")).toBe(fixture.fixture);
    expect(readFileSync(fixture.captures.bunArgs, "utf8")).toBe("--help\n");
    expect(existsSync(readFileSync(fixture.captures.bunScript, "utf8").trim())).toBe(false);
    expect(existsSync(fixture.captures.nodeScript)).toBe(false);
  } finally {
    rmSync(fixture.tempDir, { recursive: true, force: true });
  }
});

test("GitHub runner falls back to Node and forwards CLI arguments", () => {
  const fixture = createRunnerFixture(["node"]);
  try {
    const result = runGitHubRunner(fixture.env);

    expect(result.status).toBe(0);
    expect(readFileSync(fixture.captures.nodeSource, "utf8")).toBe(fixture.fixture);
    expect(readFileSync(fixture.captures.nodeArgs, "utf8")).toBe("--help\n");
    expect(existsSync(readFileSync(fixture.captures.nodeScript, "utf8").trim())).toBe(false);
    expect(existsSync(fixture.captures.bunScript)).toBe(false);
  } finally {
    rmSync(fixture.tempDir, { recursive: true, force: true });
  }
});
