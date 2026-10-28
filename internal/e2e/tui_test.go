// Package e2e_test drives the built agent-links binary through a real
// pseudo-terminal, covering the interactive Bubble Tea paths: the picker,
// the plan confirmation, and the source-file setup flow.
//
// The primary driver is python3's pty.fork(): it feeds keys only after waiting
// for on-screen markers, so it also covers progress through the flows. When
// python3 is unavailable (or too old for waitstatus_to_exitcode), the tests
// fall back to pre-feeding every keystroke through script(1), trying the BSD
// and util-linux argument forms; when neither driver exists the tests skip
// rather than fail.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var binaryPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "agent-links-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binaryPath = filepath.Join(dir, "agent-links")
	build := exec.Command("go", "build", "-o", binaryPath, "github.com/johnnymayodev/Global-AGENTS.md/cmd/agent-links")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s", err, output)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// --- driver plumbing ---------------------------------------------------

type tuiStep struct {
	wait string
	send string

	// delay pauses after sending the keys. Escape followed immediately by
	// another key can decode as Alt+key, so multi-step back navigation gives
	// the terminal time to deliver the lone escape first. Only the python
	// driver honors it.
	delay time.Duration
}

type tuiSpec struct {
	// home is the isolated HOME the binary runs with, and the working
	// directory for both drivers.
	home string

	// path overrides the /nonexistent PATH; the piped-script test needs the
	// shell's own utilities available.
	path string

	// exec overrides the command run on the pseudo-terminal; the default is
	// the built binary with args.
	exec []string
	args []string

	// keys is pre-fed on the script(1) path and used for diagnostics.
	keys []string

	// steps wait for each marker before sending the next keys (python).
	steps []tuiStep

	// signal optionally sends a signal to the child once wait appears.
	signal     string
	signalWait string
}

type tuiResult struct {
	output   string
	exitCode int

	// lastStepMS is the time from the final key write to the child's exit.
	// Only measured by the python driver; script(1) pre-feeds every key.
	lastStepMS       float64
	lastStepMeasured bool

	// cookedAfter reports that the pty was back in cooked mode (matching a
	// fresh pty) after the process exited. Only checked by the python driver.
	cookedChecked bool
	cookedAfter   bool
}

func (r tuiResult) assertContains(t *testing.T, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(r.output, want) {
			t.Errorf("output does not contain %q\n--- output ---\n%s", want, r.output)
		}
	}
}

func (r tuiResult) assertNotContains(t *testing.T, unwanted ...string) {
	t.Helper()
	for _, want := range unwanted {
		if strings.Contains(r.output, want) {
			t.Errorf("output unexpectedly contains %q\n--- output ---\n%s", want, r.output)
		}
	}
}

// requirePython skips tests that need marker-by-marker key delivery or timing;
// script(1) pre-feeds every key and cannot control either.
func requirePython(t *testing.T) {
	t.Helper()
	if !pythonOK {
		t.Skip("this test needs the python3 pty driver")
	}
}

func pythonAvailable() bool {
	if exec.Command("python3", "-c", "import os, pty, select, termios; assert os.waitstatus_to_exitcode(0) == 0").Run() != nil {
		return false
	}
	return true
}

// scriptForms are the BSD and util-linux spellings of `script`. Each entry
// renders the argv for running cmd (argv form for BSD, a shell command line
// for util-linux) and captures the child's exit status.
var scriptForms = []struct {
	name      string
	argv      func(cmd []string) []string
	shellLine func(line string) []string
}{
	{
		name: "bsd",
		argv: func(cmd []string) []string { return append([]string{"-q", "/dev/null"}, cmd...) },
		shellLine: func(line string) []string {
			return []string{"-q", "/dev/null", "sh", "-c", line}
		},
	},
	{
		name: "util-linux",
		argv: func(cmd []string) []string { return []string{"-qec", shellQuote(cmd), "/dev/null"} },
		shellLine: func(line string) []string {
			return []string{"-qec", line, "/dev/null"}
		},
	},
}

func shellQuote(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// pickScriptForm probes the two script(1) argument forms and returns the
// first that runs successfully, or nil when script is unavailable.
func pickScriptForm() *struct {
	name      string
	argv      func(cmd []string) []string
	shellLine func(line string) []string
} {
	for i := range scriptForms {
		form := &scriptForms[i]
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, "script", form.argv([]string{"true"})...)
		cmd.Stdin = strings.NewReader("")
		err := cmd.Run()
		cancel()
		if err == nil {
			return form
		}
	}
	return nil
}

var (
	pythonOK   = pythonAvailable()
	scriptForm = pickScriptForm()
)

// runTUI runs spec on a real pseudo-terminal and returns the captured
// terminal output. It uses python3 when present, then script(1), and skips
// the test when neither driver is available.
func runTUI(t *testing.T, spec tuiSpec) tuiResult {
	t.Helper()
	if pythonOK {
		return runViaPython(t, spec)
	}
	if scriptForm != nil {
		return runViaScript(t, spec)
	}
	t.Skip("python3 and script(1) are unavailable; skipping TUI e2e")
	return tuiResult{}
}

// runViaPython drives the command with a pty.fork() driver script that waits
// for markers before sending keys.
func runViaPython(t *testing.T, spec tuiSpec) tuiResult {
	t.Helper()
	dir := t.TempDir()
	outputFile := filepath.Join(dir, "output")
	resultFile := filepath.Join(dir, "result.json")
	specFile := filepath.Join(dir, "spec.json")
	scriptFile := filepath.Join(dir, "driver.py")

	execArgs := spec.exec
	if execArgs == nil {
		execArgs = append([]string{binaryPath}, spec.args...)
	}
	payload := map[string]any{
		"exec":        execArgs,
		"output_file": outputFile,
		"result_file": resultFile,
		"timeout":     20,
	}
	if len(spec.steps) > 0 {
		steps := make([]map[string]any, len(spec.steps))
		for i, step := range spec.steps {
			encoded := map[string]any{"wait": step.wait, "send": step.send}
			if step.delay > 0 {
				encoded["delay"] = step.delay.Seconds()
			}
			steps[i] = encoded
		}
		payload["steps"] = steps
	}
	if spec.signal != "" {
		payload["signal"] = map[string]string{"wait": spec.signalWait, "signal": spec.signal}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specFile, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptFile, []byte(ptyDriver), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	driver := exec.CommandContext(ctx, "python3", scriptFile, specFile)
	driver.Dir = spec.home
	driver.Env = withEnv(spec.home, spec.path)
	var driverErr bytes.Buffer
	driver.Stderr = &driverErr
	if err := driver.Run(); err != nil {
		t.Fatalf("pty driver failed: %v\n%s", err, driverErr.String())
	}

	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		ExitCode         int     `json:"exit_code"`
		TimedOut         bool    `json:"timed_out"`
		CookedChecked    bool    `json:"cooked_checked"`
		CookedAfter      bool    `json:"cooked_after"`
		LastStepMS       float64 `json:"last_step_ms"`
		LastStepMeasured bool    `json:"last_step_measured"`
	}
	resultData, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(resultData, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.TimedOut {
		t.Fatalf("the session did not finish within the driver timeout\n--- output ---\n%s", output)
	}
	return tuiResult{
		output:           string(output),
		exitCode:         decoded.ExitCode,
		lastStepMS:       decoded.LastStepMS,
		lastStepMeasured: decoded.LastStepMeasured,
		cookedChecked:    decoded.CookedChecked,
		cookedAfter:      decoded.CookedAfter,
	}
}

// runViaScript pre-feeds every keystroke through script(1); it cannot inspect
// the terminal state after exit or send signals mid-run.
func runViaScript(t *testing.T, spec tuiSpec) tuiResult {
	t.Helper()
	keys := strings.Join(spec.keys, "")
	if len(spec.steps) > 0 {
		keys = ""
		for _, step := range spec.steps {
			keys += step.send
		}
	}

	var args []string
	if spec.exec == nil {
		args = scriptForm.argv(append([]string{binaryPath}, spec.args...))
	} else {
		args = scriptForm.shellLine(shellQuote(spec.exec))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "script", args...)
	cmd.Dir = spec.home
	cmd.Env = withEnv(spec.home, spec.path)
	cmd.Stdin = strings.NewReader(keys)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	timedOut := ctx.Err() != nil
	if timedOut {
		t.Fatalf("the session did not finish within the driver timeout\n--- output ---\n%s", output.String())
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// The child's exit status is a normal outcome.
			return tuiResult{output: output.String(), exitCode: exitErr.ExitCode()}
		}
		t.Skipf("script(1) is not usable here: %v", err)
	}
	return tuiResult{output: output.String(), exitCode: 0}
}

// ptyDriver is the python3 pty.fork() driver. It reads a JSON spec, runs the
// command on a pseudo-terminal, feeds keys once their wait markers appear,
// and writes the raw session output plus the exit status and the terminal's
// post-exit mode to files.
const ptyDriver = `
import json, os, pty, select, signal, sys, termios, time

spec = json.load(open(sys.argv[1]))
timeout = spec.get("timeout", 20)

# A throwaway pty pair gives the cooked-mode termios of a fresh terminal for
# the after-exit comparison.
base_master, base_slave = pty.openpty()
base_attrs = termios.tcgetattr(base_master)
os.close(base_master)
os.close(base_slave)

command = spec["exec"]
pid, master = pty.fork()
if pid == 0:
    os.execv(command[0], command)

steps = list(spec.get("steps") or [])
signal_spec = spec.get("signal")
signal_sent = False
output = bytearray()
# seen is the fresh-output offset: each step must see its wait marker after
# the previous key was sent, so a marker from an earlier visit does not count.
seen = 0
deadline = time.monotonic() + timeout
status = None
timed_out = False
last_send_at = None
exit_at = None

while True:
    if time.monotonic() > deadline:
        timed_out = True
        os.kill(pid, signal.SIGKILL)
        _, status = os.waitpid(pid, 0)
        exit_at = time.monotonic()
        break

    ready, _, _ = select.select([master], [], [], 0.1)
    if ready:
        try:
            chunk = os.read(master, 4096)
        except OSError:
            chunk = b""
        output.extend(chunk)

    if steps and steps[0]["wait"].encode() in output[seen:]:
        os.write(master, steps[0]["send"].encode())
        last_send_at = time.monotonic()
        seen = len(output)
        delay = float(steps[0].get("delay", 0) or 0)
        steps.pop(0)
        if delay > 0:
            time.sleep(delay)

    if signal_spec and not signal_sent and signal_spec["wait"].encode() in output:
        os.kill(pid, getattr(signal, "SIG" + signal_spec["signal"]))
        signal_sent = True

    child, child_status = os.waitpid(pid, os.WNOHANG)
    if child:
        exit_at = time.monotonic()
        status = child_status
        break

# Drain any trailing output.
while True:
    ready, _, _ = select.select([master], [], [], 0.05)
    if not ready:
        break
    try:
        chunk = os.read(master, 4096)
    except OSError:
        break
    if not chunk:
        break
    output.extend(chunk)

attrs = termios.tcgetattr(master)
os.close(master)

with open(spec["output_file"], "wb") as f:
    f.write(output)

cooked_checked = True
cooked_after = (bool(attrs[3] & termios.ECHO) == bool(base_attrs[3] & termios.ECHO)
                and bool(attrs[3] & termios.ICANON) == bool(base_attrs[3] & termios.ICANON))
result = {
    "exit_code": os.waitstatus_to_exitcode(status),
    "timed_out": timed_out,
    "cooked_checked": cooked_checked,
    "cooked_after": cooked_after,
    "last_step_measured": last_send_at is not None,
    "last_step_ms": 0.0 if last_send_at is None else (exit_at - last_send_at) * 1000.0,
}
with open(spec["result_file"], "w") as f:
    json.dump(result, f)
`

// --- fixtures ----------------------------------------------------------

// tuiEnv is the isolated environment the driven binary runs with: a temp HOME
// and (by default) no detectable executables.
func tuiEnv(home, path string) []string {
	if path == "" {
		path = "/nonexistent"
	}
	return []string{
		"HOME=" + home,
		"PATH=" + path,
		"TERM=xterm-256color",
		"CODEX_HOME=" + filepath.Join(home, ".codex"),
		"HERMES_HOME=" + filepath.Join(home, ".hermes"),
		"PI_CONFIG_DIR=" + filepath.Join(home, ".omp"),
		"PI_CODING_AGENT_DIR=" + filepath.Join(home, ".omp", "agent"),
	}
}

func sourceFilePath(home string) string {
	return filepath.Join(home, ".config", "agents", "AGENTS.md")
}

func skillsRoot(home string) string {
	return filepath.Join(home, ".config", "agents", "skills")
}

// writeSkill creates a canonical skill directory with SKILL.md and one
// supporting file, returning the skill directory.
func writeSkill(t *testing.T, home, name string) string {
	t.Helper()
	dir := filepath.Join(skillsRoot(home), name)
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "run.sh"), []byte("#!/bin/sh\necho "+name+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeSource(t *testing.T, home string) string {
	t.Helper()
	source := sourceFilePath(home)
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("# Shared instructions\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return source
}

// withEnv returns a copy of the process environment with the TUI overrides
// applied, so python3 and sh can still find their own runtime while the
// binary sees the isolated HOME and PATH.
func withEnv(home, path string) []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "HOME", "PATH", "TERM", "CODEX_HOME", "HERMES_HOME", "PI_CONFIG_DIR", "PI_CODING_AGENT_DIR", "COLUMNS", "LINES":
			continue
		}
		env = append(env, entry)
	}
	return append(env, tuiEnv(home, path)...)
}

func allTargets(home string) []string {
	return []string{
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".pi", "agent", "AGENTS.md"),
		filepath.Join(home, ".omp", "agent", "AGENTS.md"),
		filepath.Join(home, ".hermes", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
	}
}

// --- tests -------------------------------------------------------------

func TestPickerReviewConfirm(t *testing.T) {
	home := t.TempDir()
	source := writeSource(t, home)

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Link your shared instructions into each harness.",
		"Apply plan:",
		"Apply this plan?",
		"Confirmation received; applying plan (create 1 symlink(s)).",
		"created symlink ~/.config/opencode/AGENTS.md",
	)
	if result.cookedChecked && !result.cookedAfter {
		t.Error("the terminal was left in raw mode after a normal exit")
	}

	target := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	link, err := os.Readlink(target)
	if err != nil || link != source {
		t.Errorf("target symlink = %q, %v; want a symlink to %s", link, err, source)
	}
}

func TestSkillsLaunchLinkTwoCellsThenRemoveOne(t *testing.T) {
	home := t.TempDir()
	alpha := writeSkill(t, home, "alpha")
	beta := writeSkill(t, home, "beta")

	// A pre-existing harness-specific skill must survive both flows.
	localOnly := filepath.Join(home, ".pi", "agent", "skills", "local-only")
	if err := os.MkdirAll(localOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localOnly, "SKILL.md"), []byte("# local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "2"},
			// alpha/opencode, then beta/codex, then review.
			{wait: "Link your shared skills into each harness.", send: " jl \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("first run exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Link your shared skills into each harness.",
		"Apply plan:",
		"create 2 symlink(s)",
		"created symlink ~/.config/opencode/skills/alpha",
		"created symlink ~/.agents/skills/beta",
	)
	if result.cookedChecked && !result.cookedAfter {
		t.Error("the terminal was left in raw mode after linking skills")
	}

	opencodeAlpha := filepath.Join(home, ".config", "opencode", "skills", "alpha")
	codexBeta := filepath.Join(home, ".agents", "skills", "beta")
	for target, source := range map[string]string{opencodeAlpha: alpha, codexBeta: beta} {
		link, err := os.Readlink(target)
		if err != nil || link != source {
			t.Errorf("%s symlink = %q, %v; want %s", target, link, err, source)
		}
	}
	// Exactly two cells, never the four-cell cross product.
	for _, target := range []string{
		filepath.Join(home, ".config", "opencode", "skills", "beta"),
		filepath.Join(home, ".agents", "skills", "alpha"),
		filepath.Join(home, ".pi", "agent", "skills", "alpha"),
		filepath.Join(home, ".claude", "skills", "alpha"),
	} {
		if _, err := os.Lstat(target); err == nil {
			t.Errorf("unexpected cross-product link: %s", target)
		}
	}

	// Supporting files are reachable through the linked directory.
	contents, err := os.ReadFile(filepath.Join(opencodeAlpha, "scripts", "run.sh"))
	if err != nil || !strings.Contains(string(contents), "alpha") {
		t.Fatalf("supporting file through the link = %q, %v", contents, err)
	}

	second := runTUI(t, tuiSpec{
		home: home,
		args: []string{"skills", "--all"},
		steps: []tuiStep{
			// The matrix starts with both managed cells selected; deselect
			// alpha/opencode only, then review.
			{wait: "Link your shared skills into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})
	if second.exitCode != 0 {
		t.Errorf("second run exit code = %d, want 0\n--- output ---\n%s", second.exitCode, second.output)
	}
	second.assertContains(t,
		"remove 1 managed symlink(s)",
		"removed managed symlink ~/.config/opencode/skills/alpha",
	)
	if second.cookedChecked && !second.cookedAfter {
		t.Error("the terminal was left in raw mode after unlinking a skill")
	}

	if _, err := os.Lstat(opencodeAlpha); err == nil {
		t.Error("the deselected skill link still exists")
	}
	if link, err := os.Readlink(codexBeta); err != nil || link != beta {
		t.Errorf("the other skill link was disturbed: %q, %v", link, err)
	}
	if _, err := os.Stat(filepath.Join(alpha, "SKILL.md")); err != nil {
		t.Errorf("canonical skill content was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localOnly, "SKILL.md")); err != nil {
		t.Errorf("an unrelated harness-specific skill was removed: %v", err)
	}
}

func TestSkillsModeIgnoresBrokenInstructionsSource(t *testing.T) {
	home := t.TempDir()
	alpha := writeSkill(t, home, "alpha")
	// A directory at the canonical instructions path is an error for the
	// instructions source, but must not block skills mode.
	if err := os.MkdirAll(sourceFilePath(home), 0o755); err != nil {
		t.Fatal(err)
	}

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "2"},
			{wait: "Link your shared skills into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t, "created symlink ~/.config/opencode/skills/alpha")
	if link, err := os.Readlink(filepath.Join(home, ".config", "opencode", "skills", "alpha")); err != nil || link != alpha {
		t.Errorf("skill link = %q, %v; want %s", link, err, alpha)
	}
}

func TestInstructionsModeIgnoresBrokenSkillsSource(t *testing.T) {
	home := t.TempDir()
	source := writeSource(t, home)
	// A regular file at the skills root is an error for the skills source,
	// but must not block instructions mode.
	if err := os.MkdirAll(filepath.Join(home, ".config", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "agents", "skills"), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t, "created symlink ~/.config/opencode/AGENTS.md")
	if link, err := os.Readlink(filepath.Join(home, ".config", "opencode", "AGENTS.md")); err != nil || link != source {
		t.Errorf("instructions link = %q, %v; want %s", link, err, source)
	}
}

func TestPickerReviewCancelAtReview(t *testing.T) {
	home := t.TempDir()
	writeSource(t, home)

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\x03"},
		},
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Apply plan:",
		"Cancelled. No changes made.",
	)
	target := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	if _, err := os.Lstat(target); err == nil {
		t.Errorf("%s exists after a cancelled review", target)
	}
}

// TestPickerSetupThenReview drives the wizard's first-run setup, its select
// step, and its review step in one session: no shared file, create a blank
// one, pick OpenCode, then apply.
func TestPickerSetupThenReview(t *testing.T) {
	home := t.TempDir()
	source := sourceFilePath(home)

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Create a blank AGENTS.md", send: "2"},
			{wait: "Link your shared instructions into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Create a blank AGENTS.md",
		fmt.Sprintf("Created blank instructions file: %s", source),
		"Apply plan:",
		"Confirmation received; applying plan (create 1 symlink(s)).",
	)

	target := filepath.Join(home, ".config", "opencode", "AGENTS.md")
	link, err := os.Readlink(target)
	if err != nil || link != source {
		t.Errorf("target symlink = %q, %v; want a symlink to %s", link, err, source)
	}
}

func TestSetupBlankThenLinkConfirm(t *testing.T) {
	home := t.TempDir()
	source := sourceFilePath(home)

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"link"},
		steps: []tuiStep{
			{wait: "Create a blank AGENTS.md", send: "2"},
			{wait: "Link plan:", send: "\r"},
		},
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Create a blank AGENTS.md",
		fmt.Sprintf("Created blank instructions file: %s", source),
		"Link plan:",
		"Apply this plan?",
		"Confirmation received; creating 6 symlink(s).",
	)

	contents, err := os.ReadFile(source)
	if err != nil || len(contents) != 0 {
		t.Fatalf("source contents = %q, %v; want empty", contents, err)
	}
	for _, target := range allTargets(home) {
		link, err := os.Readlink(target)
		if err != nil || link != source {
			t.Errorf("%s symlink = %q, %v; want a symlink to %s", target, link, err, source)
		}
	}
}

func TestSetupCopy(t *testing.T) {
	home := t.TempDir()
	source := sourceFilePath(home)
	selected := filepath.Join(home, "notes", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected, []byte("# Existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runTUI(t, tuiSpec{
		home: home,
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Copy an existing instructions file", send: "1"},
			{wait: "Path to the existing instructions file", send: selected},
			{wait: "✓ file exists", send: "\r"},
			// With no detected harnesses the empty select screen stays
			// recoverable; cancel it explicitly.
			{wait: "(none detected)", send: "\x03"},
		},
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Copy an existing instructions file",
		"Path to the existing instructions file",
		"✓ file exists",
		fmt.Sprintf("Created %s from %s", source, selected),
		"(none detected)",
		"No supported harnesses were detected.",
		"Use --all",
		"Cancelled. No links were changed.",
	)
	result.assertNotContains(t, "Cancelled. No changes made.")

	contents, err := os.ReadFile(source)
	if err != nil || string(contents) != "# Existing\n" {
		t.Fatalf("source contents = %q, %v", contents, err)
	}
}

func TestBackNavigationInstructionsAndSkills(t *testing.T) {
	requirePython(t)
	home := t.TempDir()
	writeSource(t, home)
	writeSkill(t, home, "alpha")

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "2"},
			{wait: "Link your shared skills into each harness.", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "q"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Link your shared instructions into each harness.",
		"Link your shared skills into each harness.",
		"Cancelled. No changes made.",
	)
}

func TestSetupCreationSkipsSetupOnReentry(t *testing.T) {
	requirePython(t)
	home := t.TempDir()

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Create a blank AGENTS.md", send: "2"},
			{wait: "Link your shared instructions into each harness.", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: "\x03"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		fmt.Sprintf("Created blank instructions file: %s", sourceFilePath(home)),
		"Cancelled. No links were changed.",
	)
	result.assertNotContains(t, "Cancelled. No changes made.")
	if got := strings.Count(result.output, "No shared instructions file found at"); got != 1 {
		t.Errorf("setup screen appeared %d times, want exactly 1\n--- output ---\n%s", got, result.output)
	}
}

func TestBrokenInstructionsModeIsRecoverable(t *testing.T) {
	requirePython(t)
	home := t.TempDir()
	alpha := writeSkill(t, home, "alpha")
	// A directory at the canonical instructions path breaks only that mode.
	if err := os.MkdirAll(sourceFilePath(home), 0o755); err != nil {
		t.Fatal(err)
	}

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Cannot prepare that mode.", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "2"},
			{wait: "Link your shared skills into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Cannot prepare that mode.",
		"canonical instructions path is",
		"created symlink ~/.config/opencode/skills/alpha",
	)
	if link, err := os.Readlink(filepath.Join(home, ".config", "opencode", "skills", "alpha")); err != nil || link != alpha {
		t.Errorf("skill link = %q, %v; want %s", link, err, alpha)
	}
}

func TestBrokenSkillsModeIsRecoverable(t *testing.T) {
	requirePython(t)
	home := t.TempDir()
	source := writeSource(t, home)
	// A file at the skills root breaks only that mode.
	if err := os.MkdirAll(filepath.Join(home, ".config", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillsRoot(home), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"--all"},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "2"},
			{wait: "Cannot prepare that mode.", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: " \r"},
			{wait: "Apply this plan?", send: "\r"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Cannot prepare that mode.",
		"shared skills path is",
		"created symlink ~/.config/opencode/AGENTS.md",
	)
	if link, err := os.Readlink(filepath.Join(home, ".config", "opencode", "AGENTS.md")); err != nil || link != source {
		t.Errorf("instructions link = %q, %v; want %s", link, err, source)
	}
}

func TestEmptySkillsModeIsRecoverable(t *testing.T) {
	requirePython(t)
	home := t.TempDir()
	writeSource(t, home)

	result := runTUI(t, tuiSpec{
		home: home,
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "2"},
			{wait: "(no shared skills found)", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "q"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"(no shared skills found)",
		"Add <skill-name>/SKILL.md",
		"No supported harnesses were detected.",
		"Cancelled. No changes made.",
	)
	// The CLI no longer appends a duplicate plain-text empty-state line.
	result.assertNotContains(t, "No shared skills found at")
}

func TestEmptyInstructionsModeIsRecoverable(t *testing.T) {
	requirePython(t)
	home := t.TempDir()
	writeSource(t, home)

	result := runTUI(t, tuiSpec{
		home: home,
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "(none detected)", send: "\x1b", delay: 100 * time.Millisecond},
			{wait: "What do you want to manage?", send: "q"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"(none detected)",
		"No supported harnesses were detected.",
		"Use --all",
		"Cancelled. No changes made.",
	)
}

func TestLinkCancelAfterSetupUsesLinkCopy(t *testing.T) {
	home := t.TempDir()

	result := runTUI(t, tuiSpec{
		home: home,
		args: []string{"link"},
		steps: []tuiStep{
			{wait: "Create a blank AGENTS.md", send: "2"},
			{wait: "Link plan:", send: "\x1b"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		fmt.Sprintf("Created blank instructions file: %s", sourceFilePath(home)),
		"Link plan:",
		"Cancelled. No links were changed.",
	)
	result.assertNotContains(t, "Cancelled. No changes made.")
	for _, target := range allTargets(home) {
		if _, err := os.Lstat(target); err == nil {
			t.Errorf("%s exists after a cancelled link review", target)
		}
	}
}

func TestPipedScriptReachesPicker(t *testing.T) {
	home := t.TempDir()
	writeSource(t, home)

	// Emulate the run.sh shape: a script piped into a shell whose stdin is a
	// pipe, reconnecting /dev/tty for the binary, which is what lets
	// `curl ... | bash` reach the TUI through a real terminal.
	dir := t.TempDir()
	piped := filepath.Join(dir, "piped.sh")
	script := fmt.Sprintf("exec %s \"$@\" </dev/tty\n", shellQuote([]string{binaryPath}))
	if err := os.WriteFile(piped, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	shellLine := fmt.Sprintf("cat %s | sh -s -- --all", shellQuote([]string{piped}))
	result := runTUI(t, tuiSpec{
		home: home,
		// The inner shell needs cat and sh from PATH.
		path: "/usr/bin:/bin",
		exec: []string{"/bin/sh", "-c", shellLine},
		steps: []tuiStep{
			// esc at the launch picker now returns to the mode menu, so this
			// cancel/timing test uses the unambiguous ctrl+c instead. It is
			// safe to pre-feed on the script(1) path because it is a plain
			// byte, not a signal.
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: "\x03"},
		},
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t,
		"Link your shared instructions into each harness.",
		"Cancelled. No changes made.",
	)
	if result.cookedChecked && !result.cookedAfter {
		t.Error("the terminal was left in raw mode after a normal exit")
	}
}

// TestPipedScriptCloseLatency drives the current run.sh shape (binary stdin
// reconnected with </dev/tty) and measures the time from the final key to
// process exit. A healthy run is ~50-150 ms; the pre-fix 500 ms Bubble Tea
// read-loop timeout showed up as ~550 ms plus driver overhead.
func TestPipedScriptCloseLatency(t *testing.T) {
	requirePython(t)
	home := t.TempDir()
	writeSource(t, home)

	dir := t.TempDir()
	piped := filepath.Join(dir, "piped.sh")
	script := fmt.Sprintf("exec %s \"$@\" </dev/tty\n", shellQuote([]string{binaryPath}))
	if err := os.WriteFile(piped, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	shellLine := fmt.Sprintf("cat %s | sh -s -- --all", shellQuote([]string{piped}))
	result := runTUI(t, tuiSpec{
		home: home,
		// The inner shell needs cat and sh from PATH.
		path: "/usr/bin:/bin",
		exec: []string{"/bin/sh", "-c", shellLine},
		steps: []tuiStep{
			{wait: "What do you want to manage?", send: "1"},
			{wait: "Link your shared instructions into each harness.", send: "\x03"},
		},
	})
	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0\n--- output ---\n%s", result.exitCode, result.output)
	}
	if !result.lastStepMeasured {
		t.Fatal("the driver did not measure the final key to exit")
	}
	t.Logf("close latency from final key to exit: %.1f ms", result.lastStepMS)
	if result.lastStepMS >= 400 {
		t.Errorf("close latency = %.1f ms, want < 400 ms (the pre-fix stall is ~550 ms)\n--- output ---\n%s",
			result.lastStepMS, result.output)
	}
	result.assertContains(t, "Cancelled. No changes made.")
}

func TestTerminalRestoredAfterSignal(t *testing.T) {
	if !pythonOK {
		t.Skip("the signal test needs the python3 pty driver")
	}
	home := t.TempDir()
	writeSource(t, home)

	result := runTUI(t, tuiSpec{
		home:       home,
		args:       []string{"--all"},
		steps:      []tuiStep{{wait: "What do you want to manage?", send: "1"}},
		signal:     "TERM",
		signalWait: "Link your shared instructions into each harness.",
	})

	if result.exitCode != 0 {
		t.Errorf("exit code = %d, want 0 (a graceful SIGTERM cancellation)\n--- output ---\n%s", result.exitCode, result.output)
	}
	result.assertContains(t, "Link your shared instructions into each harness.", "Cancelled. No changes made.")

	// The renderer must exit the alternate screen and re-enable the cursor
	// before the process ends.
	if !strings.Contains(result.output, "\x1b[?1049l") {
		t.Error("the alternate screen was not exited")
	}
	lastHide := strings.LastIndex(result.output, "\x1b[?25l")
	lastShow := strings.LastIndex(result.output, "\x1b[?25h")
	if lastHide == -1 || lastShow == -1 || lastShow < lastHide {
		t.Error("the cursor was left hidden")
	}
	if result.cookedChecked && !result.cookedAfter {
		t.Error("the terminal was left in raw mode after SIGTERM")
	}
}
