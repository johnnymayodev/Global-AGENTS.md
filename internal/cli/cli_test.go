package cli_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/cli"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// updateGolden rewrites the golden files with -update.
var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

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
	{name: "picker_none", fixture: writeSource, stdin: constStdin("1\n")},
	{name: "picker_claude", fixture: sourceAndClaudeDir, stdin: constStdin("1\n")},
	{name: "picker_all", args: []string{"--all"}, fixture: writeSource, stdin: constStdin("1\n")},
	{name: "launch_invalid", stdin: constStdin("x\nq\n")},
	{name: "launch_skills_empty", stdin: constStdin("2\n")},
	{name: "skills_empty", args: []string{"skills", "--list"}},
	{name: "skills_list", args: []string{"skills", "--list", "--all"}, fixture: skillsWithManagedAndConflict},
	{name: "skills_link_yes", args: []string{"skills", "link", "git-release"}, fixture: writeSkills, stdin: constStdin("yes\n")},
	{name: "skills_link_unavailable", args: []string{"skills", "link", "synced"}, fixture: writeSkills, stdin: constStdin("yes\n")},
	{name: "skills_link_conflict", args: []string{"skills", "link", "git-release"}, fixture: skillsWithLinkConflict},
	{name: "skills_link_missing", args: []string{"skills", "link", "missing"}, fixture: writeSkills},
	{name: "skills_unlink_managed", args: []string{"skills", "unlink", "git-release"}, fixture: skillsWithManagedLink},
	{name: "skills_invalid_name", args: []string{"skills", "unlink", "../escape"}},
	{name: "skills_unknown", args: []string{"skills", "bogus"}},
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

			if want := readGoldenExit(t, sc.name, code); code != want {
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
		filepath.Join(home, ".config", "muse", "AGENTS.md"),
		filepath.Join(home, ".grok", "AGENTS.md"),
		filepath.Join(home, ".gemini", "AGENTS.md"),
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

func writeSkills(t *testing.T, home string) {
	t.Helper()
	root := filepath.Join(home, ".config", "agents", "skills")
	for _, name := range []string{"git-release", "pdf-tools", "synced"} {
		mustMkdirAll(t, filepath.Join(root, name))
		mustWriteFile(t, filepath.Join(root, name, "SKILL.md"), "# "+name+"\n")
	}
	mustMkdirAll(t, filepath.Join(root, "git-release", "scripts"))
	mustWriteFile(t, filepath.Join(root, "git-release", "scripts", "release.sh"), "#!/bin/sh\n")
}

func skillsWithManagedLink(t *testing.T, home string) {
	t.Helper()
	writeSkills(t, home)
	mustMkdirAll(t, filepath.Join(home, ".config", "opencode", "skills"))
	mustSymlink(t,
		filepath.Join(home, ".config", "agents", "skills", "git-release"),
		filepath.Join(home, ".config", "opencode", "skills", "git-release"))
}

func skillsWithLinkConflict(t *testing.T, home string) {
	t.Helper()
	writeSkills(t, home)
	mustMkdirAll(t, filepath.Join(home, ".claude", "skills", "git-release"))
}

func skillsWithManagedAndConflict(t *testing.T, home string) {
	t.Helper()
	skillsWithManagedLink(t, home)
	mustMkdirAll(t, filepath.Join(home, ".claude", "skills", "pdf-tools"))
}

func TestSkillsEmptyDoesNotCreateRoot(t *testing.T) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := cli.RunWith([]string{"skills", "--list"}, cli.Options{
		LookupEnv: lookupEnv(scenarioEnv(home)),
		Cwd:       home,
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No shared skills found at") {
		t.Errorf("stdout = %q, want the empty-state guidance", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "agents", "skills")); !os.IsNotExist(err) {
		t.Errorf("the skills root was created or could not be checked: %v", err)
	}
}

func TestModesDoNotRequireEachOther(t *testing.T) {
	t.Run("skills mode works without AGENTS.md", func(t *testing.T) {
		home := t.TempDir()
		writeSkills(t, home)
		var stdout, stderr bytes.Buffer
		code := cli.RunWith([]string{"skills", "--list", "--all"}, cli.Options{
			LookupEnv: lookupEnv(scenarioEnv(home)),
			Cwd:       home,
			Stdin:     strings.NewReader(""),
			Stdout:    &stdout,
			Stderr:    &stderr,
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Shared skills:") {
			t.Errorf("skills table missing:\n%s", stdout.String())
		}
		if _, err := os.Stat(filepath.Join(home, ".config", "agents", "AGENTS.md")); !os.IsNotExist(err) {
			t.Errorf("instructions file was created by skills mode: %v", err)
		}
	})

	t.Run("instructions mode works without a skills root", func(t *testing.T) {
		home := t.TempDir()
		writeSource(t, home)
		var stdout, stderr bytes.Buffer
		code := cli.RunWith([]string{"--list"}, cli.Options{
			LookupEnv: lookupEnv(scenarioEnv(home)),
			Cwd:       home,
			Stdin:     strings.NewReader(""),
			Stdout:    &stdout,
			Stderr:    &stderr,
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Global instructions:") {
			t.Errorf("instructions table missing:\n%s", stdout.String())
		}
	})
}

func TestSkillsRootFileIsAnError(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "agents", "skills"), []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := cli.RunWith([]string{"skills", "--list"}, cli.Options{
		LookupEnv: lookupEnv(scenarioEnv(home)),
		Cwd:       home,
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "shared skills path is not a directory") {
		t.Errorf("stderr = %q, want a not-a-directory error", stderr.String())
	}
}

func TestSkillsLinkConflictMakesNoChanges(t *testing.T) {
	home := t.TempDir()
	skillsWithLinkConflict(t, home)
	var stdout, stderr bytes.Buffer
	code := cli.RunWith([]string{"skills", "link", "git-release"}, cli.Options{
		LookupEnv: lookupEnv(scenarioEnv(home)),
		Cwd:       home,
		Stdin:     strings.NewReader("yes\n"),
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing to replace existing path") {
		t.Errorf("stderr = %q, want a conflict error", stderr.String())
	}
	for _, target := range []string{
		filepath.Join(home, ".config", "opencode", "skills", "git-release"),
		filepath.Join(home, ".agents", "skills", "git-release"),
		filepath.Join(home, ".pi", "agent", "skills", "git-release"),
		filepath.Join(home, ".omp", "agent", "skills", "git-release"),
		filepath.Join(home, ".hermes", "skills", "git-release"),
	} {
		if _, err := os.Lstat(target); err == nil {
			t.Errorf("a conflicting plan created %s before aborting", target)
		}
	}
}

func TestSkillsListIsReadOnly(t *testing.T) {
	home := t.TempDir()
	skillsWithManagedAndConflict(t, home)
	var stdout, stderr bytes.Buffer
	code := cli.RunWith([]string{"skills", "--list", "--all"}, cli.Options{
		LookupEnv: lookupEnv(scenarioEnv(home)),
		Cwd:       home,
		Stdin:     strings.NewReader(""),
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	managed := filepath.Join(home, ".config", "opencode", "skills", "git-release")
	if dest, err := os.Readlink(managed); err != nil || dest != filepath.Join(home, ".config", "agents", "skills", "git-release") {
		t.Errorf("managed link changed: %q, %v", dest, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".agents", "skills", "git-release")); err == nil {
		t.Error("skills --list created a link")
	}
}

func assertGolden(t *testing.T, name, got, home string) {
	t.Helper()
	if *updateGolden {
		if err := os.WriteFile(filepath.Join("testdata", name), []byte(strings.ReplaceAll(got, home, "{{HOME}}")), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(string(data), "{{HOME}}", home)
	if got != want {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func readGoldenExit(t *testing.T, name string, got int) int {
	t.Helper()
	if *updateGolden {
		path := filepath.Join("testdata", name+".exit")
		if err := os.WriteFile(path, []byte(strconv.Itoa(got)), 0o644); err != nil {
			t.Fatal(err)
		}
		return got
	}
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
