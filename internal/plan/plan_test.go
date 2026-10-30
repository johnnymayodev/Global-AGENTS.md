package plan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

func testContext(t *testing.T) harness.Context {
	t.Helper()
	home := t.TempDir()
	ctx, err := harness.BuildContext(func(key string) (string, bool) {
		if key == "HOME" {
			return home, true
		}
		return "", false
	}, home)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func writeSource(t *testing.T, ctx harness.Context) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ctx.SourceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ctx.SourceFile, []byte("shared\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testHarness(id, target string) harness.Harness {
	return harness.Harness{ID: id, Name: id, Target: target}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLinks(t *testing.T) {
	t.Run("classifies targets and deduplicates by target", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		dir := t.TempDir()

		toCreate := testHarness("a", filepath.Join(dir, "a", "AGENTS.md"))
		managed := testHarness("m", filepath.Join(dir, "m", "AGENTS.md"))
		symlink(t, ctx.SourceFile, managed.Target)

		plan, err := Links([]harness.Harness{toCreate, managed, toCreate}, ctx)
		if err != nil {
			t.Fatalf("Links: %v", err)
		}
		if len(plan.ToCreate) != 1 || plan.ToCreate[0].ID != "a" {
			t.Errorf("ToCreate = %v", plan.ToCreate)
		}
		if len(plan.AlreadyLinked) != 1 || plan.AlreadyLinked[0].ID != "m" {
			t.Errorf("AlreadyLinked = %v", plan.AlreadyLinked)
		}
	})

	t.Run("no targets needs no source file", func(t *testing.T) {
		ctx := testContext(t)
		plan, err := Links(nil, ctx)
		if err != nil {
			t.Fatalf("Links: %v", err)
		}
		if len(plan.ToCreate) != 0 || len(plan.AlreadyLinked) != 0 {
			t.Errorf("plan = %+v", plan)
		}
	})

	t.Run("missing source file", func(t *testing.T) {
		ctx := testContext(t)
		h := testHarness("a", filepath.Join(t.TempDir(), "AGENTS.md"))
		_, err := Links([]harness.Harness{h}, ctx)
		want := "missing canonical instructions file: " + ctx.SourceFile + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("Links error = %v, want %q", err, want)
		}
	})

	t.Run("refuses unrelated symlink", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		h := testHarness("a", filepath.Join(t.TempDir(), "AGENTS.md"))
		symlink(t, "/elsewhere/target.md", h.Target)

		_, err := Links([]harness.Harness{h}, ctx)
		want := "refusing to replace unrelated symlink: " + h.Target + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("Links error = %v, want %q", err, want)
		}
	})

	t.Run("refuses existing path", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		h := testHarness("a", filepath.Join(t.TempDir(), "AGENTS.md"))
		writeFile(t, h.Target)

		_, err := Links([]harness.Harness{h}, ctx)
		want := "refusing to replace existing path: " + h.Target + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("Links error = %v, want %q", err, want)
		}
	})
}

func TestUnlinks(t *testing.T) {
	ctx := testContext(t)
	dir := t.TempDir()

	managed := testHarness("m", filepath.Join(dir, "m", "AGENTS.md"))
	symlink(t, ctx.SourceFile, managed.Target)
	absent := testHarness("a", filepath.Join(dir, "a", "AGENTS.md"))
	file := testHarness("f", filepath.Join(dir, "f", "AGENTS.md"))
	writeFile(t, file.Target)
	unrelated := testHarness("u", filepath.Join(dir, "u", "AGENTS.md"))
	symlink(t, "/elsewhere/target.md", unrelated.Target)

	plan, err := Unlinks([]harness.Harness{managed, absent, file, unrelated, managed}, ctx)
	if err != nil {
		t.Fatalf("Unlinks: %v", err)
	}
	if len(plan.ToRemove) != 1 || plan.ToRemove[0].ID != "m" {
		t.Errorf("ToRemove = %v", plan.ToRemove)
	}
	if len(plan.AlreadyAbsent) != 1 || plan.AlreadyAbsent[0].ID != "a" {
		t.Errorf("AlreadyAbsent = %v", plan.AlreadyAbsent)
	}
	if len(plan.Untouched) != 2 || plan.Untouched[0].ID != "f" || plan.Untouched[1].ID != "u" {
		t.Errorf("Untouched = %v", plan.Untouched)
	}

	t.Run("managed links are recognized without the source file", func(t *testing.T) {
		ctx := testContext(t)
		h := testHarness("m", filepath.Join(dir, "gone", "AGENTS.md"))
		symlink(t, ctx.SourceFile, h.Target)
		plan, err := Unlinks([]harness.Harness{h}, ctx)
		if err != nil {
			t.Fatalf("Unlinks: %v", err)
		}
		if len(plan.ToRemove) != 1 {
			t.Fatalf("ToRemove = %v", plan.ToRemove)
		}
	})
}

func TestCreateLinks(t *testing.T) {
	t.Run("creates missing directories and links", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		h := testHarness("opencode", filepath.Join(ctx.Home, ".config", "opencode", "AGENTS.md"))

		var stdout, stderr bytes.Buffer
		if err := CreateLinks([]harness.Harness{h}, ctx, &stdout, &stderr); err != nil {
			t.Fatalf("CreateLinks: %v", err)
		}
		if !strings.Contains(stdout.String(), "created directory ~/.config\n") {
			t.Errorf("stdout = %q, want a created directory line for the first created directory", stdout.String())
		}
		if !strings.Contains(stdout.String(), "created symlink ~/.config/opencode/AGENTS.md\n") {
			t.Errorf("stdout = %q, want a created symlink line", stdout.String())
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want empty", stderr.String())
		}
		dest, err := os.Readlink(h.Target)
		if err != nil || dest != ctx.SourceFile {
			t.Errorf("readlink = %q, %v; want %q", dest, err, ctx.SourceFile)
		}
	})

	t.Run("reports the first directory that was created", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		h := testHarness("deep", filepath.Join(ctx.Home, "deep", "nested", "AGENTS.md"))

		var stdout, stderr bytes.Buffer
		if err := CreateLinks([]harness.Harness{h}, ctx, &stdout, &stderr); err != nil {
			t.Fatalf("CreateLinks: %v", err)
		}
		if !strings.Contains(stdout.String(), "created directory ~/deep\n") {
			t.Errorf("stdout = %q, want the topmost created directory", stdout.String())
		}
	})

	t.Run("already linked targets are reported and skipped", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		h := testHarness("opencode", filepath.Join(ctx.Home, ".config", "opencode", "AGENTS.md"))
		symlink(t, ctx.SourceFile, h.Target)

		var stdout, stderr bytes.Buffer
		if err := CreateLinks([]harness.Harness{h}, ctx, &stdout, &stderr); err != nil {
			t.Fatalf("CreateLinks: %v", err)
		}
		if !strings.Contains(stdout.String(), "already linked ~/.config/opencode/AGENTS.md\n") {
			t.Errorf("stdout = %q, want an already linked line", stdout.String())
		}
	})

	t.Run("destination changed after planning", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		h := testHarness("opencode", filepath.Join(ctx.Home, ".config", "opencode", "AGENTS.md"))
		writeFile(t, h.Target)

		var stdout, stderr bytes.Buffer
		err := CreateLinks([]harness.Harness{h}, ctx, &stdout, &stderr)
		want := "destination changed after planning; refusing to overwrite: " + h.Target
		if err == nil || err.Error() != want {
			t.Fatalf("CreateLinks error = %v, want %q", err, want)
		}
		wantStop := "stopped at " + h.Target + "; earlier reported changes remain, and later destinations were not attempted.\n"
		if stderr.String() != wantStop {
			t.Errorf("stderr = %q, want %q", stderr.String(), wantStop)
		}
	})

	t.Run("failure after directory creation reports a partial directory", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		blocker := filepath.Join(t.TempDir(), "blocker")
		writeFile(t, blocker)
		h := testHarness("broken", filepath.Join(blocker, "sub", "AGENTS.md"))

		var stdout, stderr bytes.Buffer
		err := CreateLinks([]harness.Harness{h}, ctx, &stdout, &stderr)
		if err == nil {
			t.Fatal("CreateLinks unexpectedly succeeded")
		}
		wantStop := "stopped at " + h.Target + "; earlier reported changes remain, directory creation for this destination may be partial, and later destinations were not attempted.\n"
		if stderr.String() != wantStop {
			t.Errorf("stderr = %q, want %q", stderr.String(), wantStop)
		}
	})

	t.Run("later destinations are not attempted after a failure", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)
		first := testHarness("first", filepath.Join(t.TempDir(), "first", "AGENTS.md"))
		blocked := testHarness("blocked", filepath.Join(t.TempDir(), "blocked", "AGENTS.md"))
		writeFile(t, blocked.Target)
		last := testHarness("last", filepath.Join(t.TempDir(), "last", "AGENTS.md"))

		var stdout, stderr bytes.Buffer
		err := CreateLinks([]harness.Harness{first, blocked, last}, ctx, &stdout, &stderr)
		if err == nil {
			t.Fatal("CreateLinks unexpectedly succeeded")
		}
		if _, statErr := os.Lstat(first.Target); statErr != nil {
			t.Errorf("first target was not created: %v", statErr)
		}
		if _, statErr := os.Lstat(last.Target); statErr == nil {
			t.Error("last target should not have been created")
		}
	})

	t.Run("destination appearing during creation is refused", func(t *testing.T) {
		ctx := testContext(t)
		dir := t.TempDir()
		target := filepath.Join(dir, "AGENTS.md")
		writeFile(t, target)

		err := createSymlink(ctx.SourceFile, target)
		want := "destination appeared after planning; refusing to overwrite: " + target
		if err == nil || err.Error() != want {
			t.Fatalf("createSymlink error = %v, want %q", err, want)
		}
	})
}

func TestRemoveManagedLinks(t *testing.T) {
	ctx := testContext(t)
	dir := t.TempDir()

	managed := testHarness("m", filepath.Join(dir, "m", "AGENTS.md"))
	symlink(t, ctx.SourceFile, managed.Target)
	absent := testHarness("a", filepath.Join(dir, "a", "AGENTS.md"))
	file := testHarness("f", filepath.Join(dir, "f", "AGENTS.md"))
	writeFile(t, file.Target)
	unrelated := testHarness("u", filepath.Join(dir, "u", "AGENTS.md"))
	symlink(t, "/elsewhere/target.md", unrelated.Target)

	var stdout, stderr bytes.Buffer
	if err := RemoveManagedLinks([]harness.Harness{managed, absent, file, unrelated}, ctx, &stdout, &stderr); err != nil {
		t.Fatalf("RemoveManagedLinks: %v", err)
	}
	if _, err := os.Lstat(managed.Target); err == nil {
		t.Error("managed target should have been removed")
	}
	if _, err := os.Lstat(file.Target); err != nil {
		t.Error("regular file should be left untouched")
	}
	if _, err := os.Lstat(unrelated.Target); err != nil {
		t.Error("unrelated symlink should be left untouched")
	}

	out := stdout.String()
	if !strings.Contains(out, "removed managed symlink "+managed.Target+"\n") {
		t.Errorf("stdout = %q, want a removed line", out)
	}
	if !strings.Contains(out, "left untouched (already absent): "+absent.Target+"\n") {
		t.Errorf("stdout = %q, want an already absent line", out)
	}
	if strings.Count(out, "left untouched (no longer a managed symlink): ") != 2 {
		t.Errorf("stdout = %q, want two no-longer-managed lines", out)
	}

	t.Run("stops and reports after a removal failure", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root; permission checks do not apply")
		}
		ctx := testContext(t)
		dir := filepath.Join(ctx.Home, ".claude")
		h := testHarness("c", filepath.Join(dir, "CLAUDE.md"))
		symlink(t, ctx.SourceFile, h.Target)
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o755) })

		var stdout, stderr bytes.Buffer
		err := RemoveManagedLinks([]harness.Harness{h}, ctx, &stdout, &stderr)
		if err == nil {
			t.Fatal("RemoveManagedLinks unexpectedly succeeded")
		}
		wantStop := "stopped at " + h.Target + "; earlier reported removals remain, and later destinations were not attempted.\n"
		if stderr.String() != wantStop {
			t.Errorf("stderr = %q, want %q", stderr.String(), wantStop)
		}
	})
}

func TestReconcile(t *testing.T) {
	ctx := testContext(t)
	writeSource(t, ctx)

	create := testHarness("create", filepath.Join(t.TempDir(), "create", "AGENTS.md"))
	remove := testHarness("remove", filepath.Join(t.TempDir(), "remove", "AGENTS.md"))
	symlink(t, ctx.SourceFile, remove.Target)
	linked := testHarness("linked", filepath.Join(t.TempDir(), "linked", "AGENTS.md"))
	symlink(t, ctx.SourceFile, linked.Target)

	visible := []harness.Harness{create, remove, linked}
	plan, err := Reconcile(visible, map[string]bool{"create": true, "linked": true}, ctx)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if len(plan.ToCreate) != 1 || plan.ToCreate[0].ID != "create" {
		t.Errorf("ToCreate = %v", plan.ToCreate)
	}
	if len(plan.AlreadyLinked) != 1 || plan.AlreadyLinked[0].ID != "linked" {
		t.Errorf("AlreadyLinked = %v", plan.AlreadyLinked)
	}
	if len(plan.ToRemove) != 1 || plan.ToRemove[0].ID != "remove" {
		t.Errorf("ToRemove = %v", plan.ToRemove)
	}

	t.Run("a target kept by a selected harness is never unlinked", func(t *testing.T) {
		shared := filepath.Join(t.TempDir(), "shared", "AGENTS.md")
		first := testHarness("first", shared)
		second := testHarness("second", shared)
		symlink(t, ctx.SourceFile, shared)

		plan, err := Reconcile([]harness.Harness{first, second}, map[string]bool{"first": true}, ctx)
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if len(plan.ToRemove) != 0 {
			t.Errorf("ToRemove = %v, want none", plan.ToRemove)
		}
		if len(plan.AlreadyLinked) != 1 || plan.AlreadyLinked[0].ID != "first" {
			t.Errorf("AlreadyLinked = %v", plan.AlreadyLinked)
		}
	})
}

func TestApplyReconcile(t *testing.T) {
	t.Run("creates before removing", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)

		remove := testHarness("remove", filepath.Join(ctx.Home, ".claude", "CLAUDE.md"))
		symlink(t, ctx.SourceFile, remove.Target)
		create := testHarness("create", filepath.Join(t.TempDir(), "create", "AGENTS.md"))
		writeFile(t, create.Target) // the destination appears after planning

		plan := ReconcilePlan{
			LinkPlan:   LinkPlan{ToCreate: []harness.Harness{create}},
			UnlinkPlan: UnlinkPlan{ToRemove: []harness.Harness{remove}},
		}

		var stdout, stderr bytes.Buffer
		err := ApplyReconcile(plan, ctx, &stdout, &stderr)
		if err == nil {
			t.Fatal("ApplyReconcile unexpectedly succeeded")
		}
		if _, statErr := os.Lstat(remove.Target); statErr != nil {
			t.Error("the managed link must survive a failed link creation")
		}
		if !strings.Contains(stderr.String(), "No planned managed links were removed because link creation did not finish.\n") {
			t.Errorf("stderr = %q, want the removal-guard message", stderr.String())
		}
	})

	t.Run("applies both sides of the plan", func(t *testing.T) {
		ctx := testContext(t)
		writeSource(t, ctx)

		remove := testHarness("remove", filepath.Join(ctx.Home, ".claude", "CLAUDE.md"))
		symlink(t, ctx.SourceFile, remove.Target)
		create := testHarness("create", filepath.Join(t.TempDir(), "create", "AGENTS.md"))

		plan := ReconcilePlan{
			LinkPlan:   LinkPlan{ToCreate: []harness.Harness{create}},
			UnlinkPlan: UnlinkPlan{ToRemove: []harness.Harness{remove}},
		}

		var stdout, stderr bytes.Buffer
		if err := ApplyReconcile(plan, ctx, &stdout, &stderr); err != nil {
			t.Fatalf("ApplyReconcile: %v", err)
		}
		if _, err := os.Lstat(remove.Target); err == nil {
			t.Error("the managed link should have been removed")
		}
		if dest, err := os.Readlink(create.Target); err != nil || dest != ctx.SourceFile {
			t.Errorf("readlink = %q, %v; want %q", dest, err, ctx.SourceFile)
		}
	})
}

func TestLinkNotes(t *testing.T) {
	t.Run("no notes", func(t *testing.T) {
		ctx := testContext(t)
		notes, err := LinkNotes([]harness.Harness{testHarness("opencode", "t")}, ctx)
		if err != nil || notes != "" {
			t.Fatalf("LinkNotes = %q, %v; want empty", notes, err)
		}
	})

	t.Run("codex override note", func(t *testing.T) {
		ctx := testContext(t)
		override := filepath.Join(ctx.CodexHome, "AGENTS.override.md")
		writeFile(t, override)
		codex := testHarness("codex", "t")

		notes, err := LinkNotes([]harness.Harness{codex}, ctx)
		if err != nil {
			t.Fatalf("LinkNotes: %v", err)
		}
		want := "note: Codex prefers its non-empty " + override + " over AGENTS.md"
		if notes != want {
			t.Errorf("notes = %q, want %q", notes, want)
		}
	})

	t.Run("empty codex override adds no note", func(t *testing.T) {
		ctx := testContext(t)
		if err := os.MkdirAll(ctx.CodexHome, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ctx.CodexHome, "AGENTS.override.md"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		notes, err := LinkNotes([]harness.Harness{testHarness("codex", "t")}, ctx)
		if err != nil || notes != "" {
			t.Fatalf("LinkNotes = %q, %v; want empty", notes, err)
		}
	})

	t.Run("hermes note with codex note joined", func(t *testing.T) {
		ctx := testContext(t)
		writeFile(t, filepath.Join(ctx.CodexHome, "AGENTS.override.md"))
		targets := []harness.Harness{testHarness("codex", "t"), testHarness("hermes", "t")}

		notes, err := LinkNotes(targets, ctx)
		if err != nil {
			t.Fatalf("LinkNotes: %v", err)
		}
		want := "note: Codex prefers its non-empty " + filepath.Join(ctx.CodexHome, "AGENTS.override.md") + " over AGENTS.md\n" +
			"note: Hermes global AGENTS.md loading depends on Hermes version;\n      current docs guarantee global SOUL.md, not AGENTS.md"
		if notes != want {
			t.Errorf("notes = %q, want %q", notes, want)
		}
	})
}
