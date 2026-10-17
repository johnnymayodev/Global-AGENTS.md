package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/cli"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// The golden files under testdata are captured from the TypeScript reference
// implementation (agent-links.ts) with {{HOME}} standing in for the temporary
// home directory. Every scenario is compared byte for byte on stdout, stderr,
// and the exit code.

type scenario struct {
	name    string
	args    []string
	fixture func(t *testing.T, home string)
	stdin   func(home string) string
}

var scenarios = []scenario{
	{name: "help", args: []string{"--help"}},
	{name: "unknown", args: []string{"bogus"}},
	{name: "home_unset", args: []string{"--list"}},
	{name: "home_relative", args: []string{"--list"}},
	{name: "list_none", args: []string{"--list"}, fixture: writeSource},
	{name: "list_all", args: []string{"--list", "--all"}, fixture: writeSource},
	{name: "list_claude", args: []string{"--list"}, fixture: sourceAndClaudeDir},
	{name: "picker_none", fixture: writeSource},
	{name: "picker_claude", fixture: sourceAndClaudeDir},
	{name: "picker_all", args: []string{"--all"}, fixture: writeSource},
	{name: "link_decline", args: []string{"link"}, fixture: writeSource, stdin: constStdin("no\n")},
	{name: "link_eof", args: []string{"link"}, fixture: writeSource},
	{name: "link_yes", args: []string{"link"}, fixture: writeSource, stdin: constStdin("yes\n")},
	{name: "link_blank", args: []string{"link"}, stdin: constStdin("2\n")},
	{name: "link_copy", args: []string{"link"}, fixture: writeCopySource, stdin: func(home string) string {
		return "1\n" + filepath.Join(home, "notes", "AGENTS.md") + "\n"
	}},
	{name: "link_copy_invalid", args: []string{"link"}, stdin: func(home string) string {
		return "1\n" + filepath.Join(home, "missing.md") + "\nq\n"
	}},
	{name: "menu_invalid", args: []string{"link"}, stdin: constStdin("x\nq\n")},
	{name: "cancel_setup", args: []string{"link"}, stdin: constStdin("q\n")},
	{name: "link_unrelated_symlink", args: []string{"link"}, fixture: sourceAndUnrelatedSymlink},
	{name: "link_existing_path", args: []string{"link"}, fixture: sourceAndExistingPath},
	{name: "link_all_linked", args: []string{"link"}, fixture: sourceAndAllLinked},
	{name: "link_codex_override", args: []string{"link"}, fixture: sourceAndCodexOverride, stdin: constStdin("yes\n")},
	{name: "unlink_none", args: []string{"unlink"}, fixture: writeSource},
	{name: "unlink_untouched", args: []string{"unlink"}, fixture: sourceAndUntouchedTarget},
	{name: "unlink_managed", args: []string{"unlink"}, fixture: sourceAndManagedLink},
}

func TestHelpWithoutHome(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		var stdout, stderr bytes.Buffer
		code := cli.RunWith([]string{arg}, cli.Options{
			LookupEnv: func(string) (string, bool) { return "", false },
			Cwd:       t.TempDir(),
			Stdin:     strings.NewReader(""),
			Stdout:    &stdout,
			Stderr:    &stderr,
		})
		if code != 0 {
			t.Errorf("%s exit code = %d, want 0", arg, code)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s stderr = %q, want empty", arg, stderr.String())
		}
		assertGolden(t, "help.stdout", stdout.String(), "")
	}
}

func TestGoldenScenarios(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			home := t.TempDir()
			if sc.fixture != nil {
				sc.fixture(t, home)
			}

			env := scenarioEnv(home)
			switch sc.name {
			case "home_unset":
				delete(env, "HOME")
			case "home_relative":
				env["HOME"] = "relative"
			}

			stdin := ""
			if sc.stdin != nil {
				stdin = sc.stdin(home)
			}

			var stdout, stderr bytes.Buffer
			code := cli.RunWith(sc.args, cli.Options{
				LookupEnv:   lookupEnv(env),
				Cwd:         home,
				Stdin:       strings.NewReader(stdin),
				Stdout:      &stdout,
				Stderr:      &stderr,
				Interactive: false,
			})

			if want := readGoldenExit(t, sc.name); code != want {
				t.Errorf("exit code = %d, want %d\nstderr:\n%s", code, want, stderr.String())
			}
			assertGolden(t, sc.name+".stdout", stdout.String(), home)
			assertGolden(t, sc.name+".stderr", stderr.String(), home)
		})
	}
}

// scenarioEnv mirrors the environment the reference suite runs the CLI with:
// an isolated home, no detectable executables, and every path override.
func scenarioEnv(home string) map[string]string {
	return map[string]string{
		"HOME":                home,
		"PATH":                "/nonexistent",
		"CODEX_HOME":          filepath.Join(home, ".codex"),
		"HERMES_HOME":         filepath.Join(home, ".hermes"),
		"PI_CONFIG_DIR":       filepath.Join(home, ".omp"),
		"OMP_PROFILE":         "",
		"PI_PROFILE":          "",
		"PI_CODING_AGENT_DIR": filepath.Join(home, ".omp", "agent"),
	}
}

func lookupEnv(env map[string]string) harness.LookupEnv {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

func constStdin(input string) func(string) string {
	return func(string) string { return input }
}

func writeSource(t *testing.T, home string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(home, ".config", "agents"))
	mustWriteFile(t, filepath.Join(home, ".config", "agents", "AGENTS.md"), "# Shared instructions\n")
}

func sourceAndClaudeDir(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	mustMkdirAll(t, filepath.Join(home, ".claude"))
}

func writeCopySource(t *testing.T, home string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(home, "notes"))
	mustWriteFile(t, filepath.Join(home, "notes", "AGENTS.md"), "# Existing instructions\nUse the shared guidelines.\n")
}

func sourceAndUnrelatedSymlink(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	mustWriteFile(t, filepath.Join(home, "other.md"), "other\n")
	mustMkdirAll(t, filepath.Join(home, ".claude"))
	mustSymlink(t, filepath.Join(home, "other.md"), filepath.Join(home, ".claude", "CLAUDE.md"))
}

func sourceAndExistingPath(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	mustMkdirAll(t, filepath.Join(home, ".claude"))
	mustWriteFile(t, filepath.Join(home, ".claude", "CLAUDE.md"), "user file\n")
}

func sourceAndManagedLink(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))
	mustSymlink(t, filepath.Join(home, ".config", "agents", "AGENTS.md"), filepath.Join(home, ".config", "opencode", "AGENTS.md"))
}

func sourceAndAllLinked(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	source := filepath.Join(home, ".config", "agents", "AGENTS.md")
	targets := []string{
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".pi", "agent", "AGENTS.md"),
		filepath.Join(home, ".omp", "agent", "AGENTS.md"),
		filepath.Join(home, ".hermes", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
	}
	for _, target := range targets {
		mustMkdirAll(t, filepath.Dir(target))
		mustSymlink(t, source, target)
	}
}

func sourceAndCodexOverride(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	mustMkdirAll(t, filepath.Join(home, ".codex"))
	mustWriteFile(t, filepath.Join(home, ".codex", "AGENTS.override.md"), "override\n")
}

func sourceAndUntouchedTarget(t *testing.T, home string) {
	t.Helper()
	writeSource(t, home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode"))
	mustWriteFile(t, filepath.Join(home, ".config", "opencode", "AGENTS.md"), "user file\n")
}

func assertGolden(t *testing.T, name, got, home string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(string(data), "{{HOME}}", home)
	if got != want {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func readGoldenExit(t *testing.T, name string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".exit"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
