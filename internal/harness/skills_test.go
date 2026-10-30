package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildContextSkillsPaths(t *testing.T) {
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": "/home/user"}), "/work")
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if ctx.SourceSkillsDir != "/home/user/.global-agents/skills" {
		t.Errorf("SourceSkillsDir = %q", ctx.SourceSkillsDir)
	}

	want := map[string]string{
		"opencode":    "/home/user/.config/opencode/skills",
		"codex":       "/home/user/.agents/skills",
		"pi":          "/home/user/.pi/agent/skills",
		"omp":         "/home/user/.omp/agent/skills",
		"hermes":      "/home/user/.hermes/skills",
		"claude":      "/home/user/.claude/skills",
		"cursor":      "/home/user/.cursor/skills",
		"muse":        "/home/user/.config/muse/skills",
		"grok":        "/home/user/.grok/skills",
		"antigravity": "/home/user/.gemini/config/skills",
	}
	for id, wantDir := range want {
		if got := harnessByID(t, ctx, id).SkillsDir; got != wantDir {
			t.Errorf("%s SkillsDir = %q, want %q", id, got, wantDir)
		}
	}
}

func TestSkillsPathsFollowEnvironmentOverrides(t *testing.T) {
	t.Run("Codex skills ignore CODEX_HOME", func(t *testing.T) {
		env := map[string]string{
			"HOME":        "/home/user",
			"CODEX_HOME":  "/custom/codex",
			"HERMES_HOME": "/custom/hermes",
		}
		ctx, err := BuildContext(lookupFrom(env), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		codex := harnessByID(t, ctx, "codex")
		if codex.Target != "/custom/codex/AGENTS.md" {
			t.Errorf("codex target = %q, want the CODEX_HOME instructions target", codex.Target)
		}
		if codex.SkillsDir != "/home/user/.agents/skills" {
			t.Errorf("codex SkillsDir = %q, want ~/.agents/skills", codex.SkillsDir)
		}
		if got := harnessByID(t, ctx, "hermes").SkillsDir; got != "/custom/hermes/skills" {
			t.Errorf("hermes SkillsDir = %q", got)
		}
	})

	t.Run("relative HERMES_HOME resolves against cwd", func(t *testing.T) {
		env := map[string]string{"HOME": "/home/user", "HERMES_HOME": "rel-hermes"}
		ctx, err := BuildContext(lookupFrom(env), "/work/dir")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		if got := harnessByID(t, ctx, "hermes").SkillsDir; got != "/work/dir/rel-hermes/skills" {
			t.Errorf("hermes SkillsDir = %q", got)
		}
	})

	t.Run("absolute overrides are kept", func(t *testing.T) {
		env := map[string]string{
			"HOME":                "/home/user",
			"HERMES_HOME":         "/abs/hermes/",
			"PI_CONFIG_DIR":       "/abs/config",
			"PI_PROFILE":          "work",
			"PI_CODING_AGENT_DIR": "/abs/agent",
		}
		ctx, err := BuildContext(lookupFrom(env), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		if got := harnessByID(t, ctx, "hermes").SkillsDir; got != "/abs/hermes/skills" {
			t.Errorf("hermes SkillsDir = %q", got)
		}
		if got := harnessByID(t, ctx, "omp").SkillsDir; got != "/abs/config/profiles/work/agent/skills" {
			t.Errorf("OMP profile SkillsDir = %q", got)
		}

		env = map[string]string{"HOME": "/home/user", "PI_CODING_AGENT_DIR": "/abs/agent"}
		ctx, err = BuildContext(lookupFrom(env), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		if got := harnessByID(t, ctx, "omp").SkillsDir; got != "/abs/agent/skills" {
			t.Errorf("OMP agent SkillsDir = %q", got)
		}
	})

	t.Run("OMP profiles and agent dirs apply to skills", func(t *testing.T) {
		env := map[string]string{"HOME": "/home/user", "PI_PROFILE": "work"}
		ctx, err := BuildContext(lookupFrom(env), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		if got := harnessByID(t, ctx, "omp").SkillsDir; got != "/home/user/.omp/profiles/work/agent/skills" {
			t.Errorf("OMP profile SkillsDir = %q", got)
		}

		env = map[string]string{"HOME": "/home/user", "PI_CODING_AGENT_DIR": "rel-agent"}
		ctx, err = BuildContext(lookupFrom(env), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		if got := harnessByID(t, ctx, "omp").SkillsDir; got != "/work/rel-agent/skills" {
			t.Errorf("OMP agent SkillsDir = %q", got)
		}
	})
}

func TestMuseFollowsXDGConfigHome(t *testing.T) {
	t.Run("absolute XDG_CONFIG_HOME", func(t *testing.T) {
		ctx, err := BuildContext(lookupFrom(map[string]string{
			"HOME":            "/home/user",
			"XDG_CONFIG_HOME": "/custom/config",
		}), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		muse := harnessByID(t, ctx, "muse")
		if got, want := muse.Target, "/custom/config/muse/AGENTS.md"; got != want {
			t.Errorf("muse target = %q, want %q", got, want)
		}
		if got, want := muse.SkillsDir, "/custom/config/muse/skills"; got != want {
			t.Errorf("muse SkillsDir = %q, want %q", got, want)
		}
	})

	t.Run("relative XDG_CONFIG_HOME is ignored", func(t *testing.T) {
		ctx, err := BuildContext(lookupFrom(map[string]string{
			"HOME":            "/home/user",
			"XDG_CONFIG_HOME": "relative/config",
		}), "/work")
		if err != nil {
			t.Fatalf("BuildContext: %v", err)
		}
		if got, want := harnessByID(t, ctx, "muse").Target, "/home/user/.config/muse/AGENTS.md"; got != want {
			t.Errorf("muse target = %q, want %q", got, want)
		}
	})
}

func TestVisibleSkillsCountsManagedLinks(t *testing.T) {
	home := t.TempDir()
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": home, "PATH": ""}), t.TempDir())
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	source := filepath.Join(ctx.SourceSkillsDir, "git-release")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("# git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codexSkills := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(codexSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, filepath.Join(codexSkills, "git-release")); err != nil {
		t.Fatal(err)
	}

	visible, err := VisibleSkills(ctx, false)
	if err != nil {
		t.Fatalf("VisibleSkills: %v", err)
	}
	if len(visible) != 1 || visible[0].ID != "codex" {
		t.Fatalf("VisibleSkills = %v, want only codex", visible)
	}

	all, err := VisibleSkills(ctx, true)
	if err != nil {
		t.Fatalf("VisibleSkills(all): %v", err)
	}
	if len(all) != len(ctx.Harnesses) {
		t.Fatalf("VisibleSkills(all) = %d harnesses, want %d", len(all), len(ctx.Harnesses))
	}
}

func TestVisibleSkillsIgnoresUnrelatedLinks(t *testing.T) {
	home := t.TempDir()
	ctx, err := BuildContext(lookupFrom(map[string]string{"HOME": home, "PATH": ""}), t.TempDir())
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	dir := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "elsewhere"), filepath.Join(dir, "other")); err != nil {
		t.Fatal(err)
	}

	visible, err := VisibleSkills(ctx, false)
	if err != nil {
		t.Fatalf("VisibleSkills: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("VisibleSkills = %v, want none", visible)
	}
}

func TestSkillUnavailableReason(t *testing.T) {
	claude := Harness{ID: "claude"}
	for _, name := range []string{"synced", "Synced", "SYNCED", "anthropic-skills", "anthropic-skills:pdf", "Anthropic-Skills:X"} {
		if got := SkillUnavailableReason(claude, name); got == "" {
			t.Errorf("SkillUnavailableReason(claude, %q) = empty, want a reason", name)
		}
	}
	for _, name := range []string{"git-release", "synced-tools", "anthropic"} {
		if got := SkillUnavailableReason(claude, name); got != "" {
			t.Errorf("SkillUnavailableReason(claude, %q) = %q, want empty", name, got)
		}
	}
	if got := SkillUnavailableReason(Harness{ID: "opencode"}, "synced"); got != "" {
		t.Errorf("opencode reason = %q, want empty", got)
	}
}
