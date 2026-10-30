package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lookupFrom(env map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}
}

func harnessByID(t *testing.T, ctx Context, id string) Harness {
	t.Helper()
	for _, h := range ctx.Harnesses {
		if h.ID == id {
			return h
		}
	}
	t.Fatalf("harness %q not found", id)
	return Harness{}
}

func TestBuildContextRequiresHome(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{name: "unset", env: map[string]string{}, wantErr: "HOME must be set"},
		{name: "empty", env: map[string]string{"HOME": ""}, wantErr: "HOME must be set"},
		{name: "relative", env: map[string]string{"HOME": "relative/path"}, wantErr: "HOME must be an absolute path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildContext(lookupFrom(tc.env), "/work")
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("BuildContext error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBuildContextDefaultPaths(t *testing.T) {
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": "/home/user"}), "/work")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}

	if ctx.Home != "/home/user" {
		t.Errorf("Home = %q", ctx.Home)
	}
	if ctx.SourceFile != "/home/user/.global-agents/AGENTS.md" {
		t.Errorf("SourceFile = %q", ctx.SourceFile)
	}
	if ctx.CodexHome != "/home/user/.codex" {
		t.Errorf("CodexHome = %q", ctx.CodexHome)
	}

	wantTargets := map[string]string{
		"opencode":    "/home/user/.config/opencode/AGENTS.md",
		"codex":       "/home/user/.codex/AGENTS.md",
		"pi":          "/home/user/.pi/agent/AGENTS.md",
		"omp":         "/home/user/.omp/agent/AGENTS.md",
		"hermes":      "/home/user/.hermes/AGENTS.md",
		"claude":      "/home/user/.claude/CLAUDE.md",
		"cursor":      "",
		"muse":        "/home/user/.config/muse/AGENTS.md",
		"grok":        "/home/user/.grok/AGENTS.md",
		"antigravity": "/home/user/.gemini/AGENTS.md",
	}
	wantOrder := []string{"claude", "codex", "cursor", "opencode", "pi", "omp", "grok", "hermes", "muse", "antigravity"}
	if len(ctx.Harnesses) != len(wantOrder) {
		t.Fatalf("len(Harnesses) = %d, want %d", len(ctx.Harnesses), len(wantOrder))
	}
	for i, id := range wantOrder {
		h := ctx.Harnesses[i]
		if h.ID != id {
			t.Errorf("Harnesses[%d].ID = %q, want %q", i, h.ID, id)
		}
		if h.Target != wantTargets[id] {
			t.Errorf("%s target = %q, want %q", id, h.Target, wantTargets[id])
		}
	}

	omp := harnessByID(t, ctx, "omp")
	if got, want := strings.Join(omp.Executables, ","), "omp,oh-my-pi"; got != want {
		t.Errorf("omp executables = %q, want %q", got, want)
	}
	if got, want := strings.Join(omp.InstallDirs, ","), "/home/user/.omp/agent,/home/user/.omp"; got != want {
		t.Errorf("omp install dirs = %q, want %q", got, want)
	}
	opencode := harnessByID(t, ctx, "opencode")
	if got, want := strings.Join(opencode.InstallDirs, ","), "/home/user/.config/opencode"; got != want {
		t.Errorf("opencode install dirs = %q, want %q", got, want)
	}
}

func TestBuildContextRelativeEnvResolved(t *testing.T) {
	env := map[string]string{
		"HOME":                "/home/user",
		"CODEX_HOME":          "rel-codex",
		"HERMES_HOME":         "rel-hermes",
		"PI_CODING_AGENT_DIR": "rel-agent",
		"PI_CONFIG_DIR":       "rel-config",
	}
	ctx, err := BuildContext(lookupFrom(env), "/work/dir")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}

	if ctx.CodexHome != "/work/dir/rel-codex" {
		t.Errorf("CodexHome = %q", ctx.CodexHome)
	}
	if got := harnessByID(t, ctx, "codex").Target; got != "/work/dir/rel-codex/AGENTS.md" {
		t.Errorf("codex target = %q", got)
	}
	if got := harnessByID(t, ctx, "hermes").Target; got != "/work/dir/rel-hermes/AGENTS.md" {
		t.Errorf("hermes target = %q", got)
	}
	// PI_CODING_AGENT_DIR resolves against the CWD; PI_CONFIG_DIR against HOME.
	if got := harnessByID(t, ctx, "omp").Target; got != "/work/dir/rel-agent/AGENTS.md" {
		t.Errorf("omp target = %q", got)
	}
	if got, want := strings.Join(harnessByID(t, ctx, "omp").InstallDirs, ","), "/work/dir/rel-agent,/home/user/rel-config"; got != want {
		t.Errorf("omp install dirs = %q, want %q", got, want)
	}
}

func TestBuildContextAbsoluteEnvKept(t *testing.T) {
	env := map[string]string{
		"HOME":                "/home/user",
		"CODEX_HOME":          "/abs/codex/",
		"HERMES_HOME":         "/abs/hermes",
		"PI_CONFIG_DIR":       "/abs/config",
		"PI_CODING_AGENT_DIR": "/abs/agent",
	}
	ctx, err := BuildContext(lookupFrom(env), "/work")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if ctx.CodexHome != "/abs/codex" {
		t.Errorf("CodexHome = %q", ctx.CodexHome)
	}
	if got := harnessByID(t, ctx, "omp").Target; got != "/abs/agent/AGENTS.md" {
		t.Errorf("omp target = %q", got)
	}
	if got := harnessByID(t, ctx, "hermes").Target; got != "/abs/hermes/AGENTS.md" {
		t.Errorf("hermes target = %q", got)
	}
}

func TestBuildContextOmpProfile(t *testing.T) {
	cases := []struct {
		name      string
		env       map[string]string
		wantAgent string
	}{
		{
			name:      "PI_PROFILE selects a profile",
			env:       map[string]string{"PI_PROFILE": "work"},
			wantAgent: "/home/user/.omp/profiles/work/agent",
		},
		{
			name:      "OMP_PROFILE wins over PI_PROFILE",
			env:       map[string]string{"OMP_PROFILE": "omp-prof", "PI_PROFILE": "pi-prof"},
			wantAgent: "/home/user/.omp/profiles/omp-prof/agent",
		},
		{
			name:      "present but empty OMP_PROFILE beats PI_PROFILE",
			env:       map[string]string{"OMP_PROFILE": "", "PI_PROFILE": "pi-prof", "PI_CODING_AGENT_DIR": "pa"},
			wantAgent: "/work/pa",
		},
		{
			name:      "empty OMP_PROFILE without other settings falls back to the default agent dir",
			env:       map[string]string{"OMP_PROFILE": "", "PI_PROFILE": "pi-prof"},
			wantAgent: "/home/user/.omp/agent",
		},
		{
			name:      "profile wins over PI_CODING_AGENT_DIR",
			env:       map[string]string{"OMP_PROFILE": "prof", "PI_CODING_AGENT_DIR": "pa"},
			wantAgent: "/home/user/.omp/profiles/prof/agent",
		},
		{
			name:      "empty PI_CODING_AGENT_DIR falls back to the default agent dir",
			env:       map[string]string{"PI_CODING_AGENT_DIR": ""},
			wantAgent: "/home/user/.omp/agent",
		},
		{
			name:      "profile directories follow a custom PI_CONFIG_DIR",
			env:       map[string]string{"PI_CONFIG_DIR": "cfg", "PI_PROFILE": "work"},
			wantAgent: "/home/user/cfg/profiles/work/agent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"HOME": "/home/user"}
			for key, value := range tc.env {
				env[key] = value
			}
			ctx, err := BuildContext(lookupFrom(env), "/work")
			if err != nil {
				t.Fatalf("BuildContext: %v", err)
			}
			if got := harnessByID(t, ctx, "omp").Target; got != filepath.Join(tc.wantAgent, "AGENTS.md") {
				t.Errorf("omp target = %q, want %q", got, filepath.Join(tc.wantAgent, "AGENTS.md"))
			}
		})
	}
}

func TestDisplayPath(t *testing.T) {
	home := "/Users/johnny"
	cases := []struct {
		path string
		want string
	}{
		{path: "/Users/johnny", want: "~"},
		{path: "/Users/johnny/", want: "~"},
		{path: "/Users/johnny/.config/opencode/AGENTS.md", want: "~/.config/opencode/AGENTS.md"},
		{path: "/Users/johnny2", want: "/Users/johnny2"},
		{path: "/Users", want: "/Users"},
		{path: "/etc/hosts", want: "/etc/hosts"},
	}
	for _, tc := range cases {
		if got := DisplayPath(tc.path, home); got != tc.want {
			t.Errorf("DisplayPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestTargetState(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.md")
	if err := os.WriteFile(source, []byte("shared"), 0o644); err != nil {
		t.Fatal(err)
	}

	writeFile := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	symlink := func(target, name string) string {
		path := filepath.Join(dir, name)
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		return path
	}

	cases := []struct {
		name   string
		target string
		want   State
	}{
		{name: "absent", target: filepath.Join(dir, "missing"), want: Absent},
		{name: "managed", target: symlink(source, "managed"), want: Managed},
		{name: "unrelated symlink", target: symlink("/elsewhere/file", "unrelated"), want: Symlink},
		{name: "relative symlink", target: symlink("other", "relative"), want: Symlink},
		{name: "regular file", target: writeFile("file.md"), want: Path},
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	cases = append(cases, struct {
		name   string
		target string
		want   State
	}{name: "directory", target: subdir, want: Path})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TargetState(tc.target, source)
			if err != nil {
				t.Fatalf("TargetState: %v", err)
			}
			if got != tc.want {
				t.Errorf("TargetState = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("dangling symlink to the source path is still managed", func(t *testing.T) {
		goneSource := filepath.Join(dir, "gone.md")
		link := symlink(goneSource, "dangling-managed")
		got, err := TargetState(link, goneSource)
		if err != nil {
			t.Fatalf("TargetState: %v", err)
		}
		if got != Managed {
			t.Errorf("TargetState = %q, want %q", got, Managed)
		}
	})
}

func TestSourceFileExists(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(path, contents string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("missing", func(t *testing.T) {
		ctx := Context{SourceFile: filepath.Join(dir, "missing", "AGENTS.md")}
		exists, err := SourceFileExists(ctx)
		if err != nil || exists {
			t.Fatalf("SourceFileExists = %v, %v; want false, nil", exists, err)
		}
	})

	t.Run("not found below a file", func(t *testing.T) {
		file := filepath.Join(dir, "plain-file")
		writeFile(file, "x")
		ctx := Context{SourceFile: filepath.Join(file, "AGENTS.md")}
		exists, err := SourceFileExists(ctx)
		if err != nil || exists {
			t.Fatalf("SourceFileExists = %v, %v; want false, nil", exists, err)
		}
	})

	t.Run("regular file", func(t *testing.T) {
		path := filepath.Join(dir, "regular", "AGENTS.md")
		writeFile(path, "shared")
		ctx := Context{SourceFile: path}
		exists, err := SourceFileExists(ctx)
		if err != nil || !exists {
			t.Fatalf("SourceFileExists = %v, %v; want true, nil", exists, err)
		}
	})

	t.Run("symlink to a file", func(t *testing.T) {
		real := filepath.Join(dir, "real", "AGENTS.md")
		writeFile(real, "shared")
		link := filepath.Join(dir, "linked", "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		ctx := Context{SourceFile: link}
		exists, err := SourceFileExists(ctx)
		if err != nil || !exists {
			t.Fatalf("SourceFileExists = %v, %v; want true, nil", exists, err)
		}
	})

	t.Run("broken symlink", func(t *testing.T) {
		link := filepath.Join(dir, "broken", "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(dir, "gone.md"), link); err != nil {
			t.Fatal(err)
		}
		ctx := Context{SourceFile: link}
		_, err := SourceFileExists(ctx)
		want := "canonical instructions path is a broken symlink: " + link + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("SourceFileExists error = %v, want %q", err, want)
		}
	})

	t.Run("symlink to a directory", func(t *testing.T) {
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(dir, "dir-link", "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(sub, link); err != nil {
			t.Fatal(err)
		}
		ctx := Context{SourceFile: link}
		_, err := SourceFileExists(ctx)
		want := "canonical instructions path is not a regular file: " + link + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("SourceFileExists error = %v, want %q", err, want)
		}
	})

	t.Run("directory at the source path", func(t *testing.T) {
		path := filepath.Join(dir, "dir-source", "AGENTS.md")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		ctx := Context{SourceFile: path}
		_, err := SourceFileExists(ctx)
		want := "canonical instructions path is not a regular file: " + path + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("SourceFileExists error = %v, want %q", err, want)
		}
	})
}

func TestAssertSourceFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, ".global-agents", "AGENTS.md")
	ctx := Context{SourceFile: source}

	err := AssertSourceFile(ctx)
	want := "missing canonical instructions file: " + source + "; no changes made"
	if err == nil || err.Error() != want {
		t.Fatalf("AssertSourceFile error = %v, want %q", err, want)
	}

	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AssertSourceFile(ctx); err != nil {
		t.Fatalf("AssertSourceFile: %v", err)
	}
}

func TestHasExecutable(t *testing.T) {
	dir := t.TempDir()
	writeExec := func(path string, mode os.FileMode) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	writeExec(filepath.Join(dir, "tool"), 0o755)
	writeExec(filepath.Join(dir, "noexec"), 0o644)
	writeExec(filepath.Join(dir, "bin", "nested"), 0o755)
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "tool"), filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		executable string
		searchPath string
		want       bool
	}{
		{name: "executable found", executable: "tool", searchPath: dir, want: true},
		{name: "symlink to executable", executable: "alias", searchPath: dir, want: true},
		{name: "not executable", executable: "noexec", searchPath: dir, want: false},
		{name: "directory is not executable file", executable: "adir", searchPath: dir, want: false},
		{name: "missing", executable: "missing", searchPath: dir, want: false},
		{name: "later entry matches", executable: "tool", searchPath: "/nonexistent:" + dir, want: true},
		{name: "not on path", executable: "tool", searchPath: "/nonexistent", want: false},
		{name: "empty PATH searches cwd", executable: "tool", searchPath: "", want: true},
		{name: "empty entry searches cwd", executable: "tool", searchPath: ":", want: true},
		{name: "relative entry resolves against cwd", executable: "nested", searchPath: "bin", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasExecutable(tc.executable, tc.searchPath, dir); got != tc.want {
				t.Errorf("HasExecutable(%q, %q) = %v, want %v", tc.executable, tc.searchPath, got, tc.want)
			}
		})
	}
}

func TestIsInstalled(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": home, "PATH": ""}), cwd)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	source := ctx.SourceFile
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("nothing installed", func(t *testing.T) {
		h := Harness{ID: "none"}
		installed, err := IsInstalled(h, ctx)
		if err != nil || installed {
			t.Fatalf("IsInstalled = %v, %v; want false, nil", installed, err)
		}
	})

	t.Run("managed target", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "AGENTS.md")
		if err := os.Symlink(source, target); err != nil {
			t.Fatal(err)
		}
		h := Harness{ID: "managed", Target: target}
		installed, err := IsInstalled(h, ctx)
		if err != nil || !installed {
			t.Fatalf("IsInstalled = %v, %v; want true, nil", installed, err)
		}
	})

	t.Run("executable on PATH", func(t *testing.T) {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "tool"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		pathCtx, err := BuildContext(lookupFrom(map[string]string{"HOME": home, "PATH": bin}), cwd)
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		h := Harness{ID: "exe", Executables: []string{"tool"}}
		installed, err := IsInstalled(h, pathCtx)
		if err != nil || !installed {
			t.Fatalf("IsInstalled = %v, %v; want true, nil", installed, err)
		}
	})

	t.Run("install directory exists", func(t *testing.T) {
		dir := t.TempDir()
		h := Harness{ID: "dir", InstallDirs: []string{dir}}
		installed, err := IsInstalled(h, ctx)
		if err != nil || !installed {
			t.Fatalf("IsInstalled = %v, %v; want true, nil", installed, err)
		}
	})
}

func TestVisible(t *testing.T) {
	home := t.TempDir()
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": home, "PATH": ""}), t.TempDir())
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}

	visible, err := Visible(ctx, false)
	if err != nil {
		t.Fatalf("Visible: %v", err)
	}
	if len(visible) != 1 || visible[0].ID != "opencode" {
		t.Fatalf("Visible = %v, want only opencode", visible)
	}

	all, err := Visible(ctx, true)
	if err != nil {
		t.Fatalf("Visible: %v", err)
	}
	if len(all) != len(WithInstructions(ctx.Harnesses)) {
		t.Fatalf("Visible(all) = %d harnesses, want %d", len(all), len(WithInstructions(ctx.Harnesses)))
	}
	for _, h := range all {
		if !h.HasInstructions() {
			t.Errorf("Visible(all) includes %s without an instructions target", h.ID)
		}
	}
}

func TestCursorIsSkillsOnly(t *testing.T) {
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": "/home/user"}), "/work")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	cursor := harnessByID(t, ctx, "cursor")
	if cursor.HasInstructions() {
		t.Errorf("cursor target = %q, want no global instructions target", cursor.Target)
	}
	if got, want := cursor.SkillsDir, "/home/user/.cursor/skills"; got != want {
		t.Errorf("cursor SkillsDir = %q, want %q", got, want)
	}
}
