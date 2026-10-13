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

test("GitHub runner downloads the entry point and forwards CLI arguments", () => {
  const tempDir = mkdtempSync(join(tmpdir(), "agent-links-runner-test-"));

  try {
    const binDir = join(tempDir, "bin");
    const fixturePath = join(tempDir, "source.ts");
    const curlUrlCapturePath = join(tempDir, "curl-url");
    const bunScriptCapturePath = join(tempDir, "bun-script");
    const bunSourceCapturePath = join(tempDir, "bun-source");
    const bunArgsCapturePath = join(tempDir, "bun-args");
    const fixture = 'console.log("fixture source");\n';

    mkdirSync(binDir);
    writeFileSync(fixturePath, fixture);

    const curlPath = join(binDir, "curl");
    writeFileSync(
      curlPath,
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
    chmodSync(curlPath, 0o755);

    const bunPath = join(binDir, "bun");
    writeFileSync(
      bunPath,
      String.raw`#!/bin/sh
set -eu
[ "$1" = "run" ]
shift
script=$1
shift
printf '%s\n' "$script" > "$BUN_SCRIPT_CAPTURE"
cp "$script" "$BUN_SOURCE_CAPTURE"
printf '%s\n' "$@" > "$BUN_ARGS_CAPTURE"
`,
    );
    chmodSync(bunPath, 0o755);

    const result = spawnSync(
      "sh",
      [join(import.meta.dir, "run.sh"), "acme/agent-links", "--help"],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          PATH: `${binDir}${delimiter}${process.env.PATH ?? ""}`,
          CURL_SOURCE_FILE: fixturePath,
          CURL_URL_CAPTURE: curlUrlCapturePath,
          BUN_SCRIPT_CAPTURE: bunScriptCapturePath,
          BUN_SOURCE_CAPTURE: bunSourceCapturePath,
          BUN_ARGS_CAPTURE: bunArgsCapturePath,
        },
      },
    );

    expect(result.status).toBe(0);
    expect(readFileSync(curlUrlCapturePath, "utf8").trim()).toBe(
      "https://raw.githubusercontent.com/acme/agent-links/main/agent-links.ts",
    );
    expect(readFileSync(bunSourceCapturePath, "utf8")).toBe(fixture);
    expect(readFileSync(bunArgsCapturePath, "utf8")).toBe("--help\n");
    expect(existsSync(readFileSync(bunScriptCapturePath, "utf8").trim())).toBe(false);
  } finally {
    rmSync(tempDir, { recursive: true, force: true });
  }
});
