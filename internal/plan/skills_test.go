package plan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

func makeSkillDir(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func skillTestHarness(id, skillsDir string) harness.Harness {
	return harness.Harness{ID: id, Name: id, SkillsDir: skillsDir}
}

func skillCell(t *testing.T, skillName string, h harness.Harness, sourceRoot string) SkillCell {
	t.Helper()
	return SkillCell{
		Skill:       skillName,
		HarnessID:   h.ID,
		HarnessName: h.Name,
		Source:      filepath.Join(sourceRoot, skillName),
		Target:      filepath.Join(h.SkillsDir, skillName),
	}
}

func TestSkillReconcileIndependentCells(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "alpha")
	beta := makeSkillDir(t, root, "beta")
	one := skillTestHarness("one", t.TempDir())
	two := skillTestHarness("two", t.TempDir())

	// beta is already linked for both harnesses.
	symlink(t, beta, filepath.Join(one.SkillsDir, "beta"))
	symlink(t, beta, filepath.Join(two.SkillsDir, "beta"))

	cells := []SkillCell{
		skillCell(t, "alpha", one, root),
		skillCell(t, "alpha", two, root),
		skillCell(t, "beta", one, root),
		skillCell(t, "beta", two, root),
	}
	selected := map[string]bool{
		CellKey("alpha", "one"): true,
		CellKey("beta", "two"):  true,
	}

	plan, err := SkillReconcile(cells, selected)
	if err != nil {
		t.Fatalf("SkillReconcile: %v", err)
	}
	if len(plan.ToCreate) != 1 || plan.ToCreate[0].Skill != "alpha" || plan.ToCreate[0].HarnessID != "one" {
		t.Errorf("ToCreate = %v, want alpha/one only", plan.ToCreate)
	}
	if len(plan.ToRemove) != 1 || plan.ToRemove[0].Skill != "beta" || plan.ToRemove[0].HarnessID != "one" {
		t.Errorf("ToRemove = %v, want beta/one only", plan.ToRemove)
	}
	if len(plan.AlreadyLinked) != 1 || plan.AlreadyLinked[0].Skill != "beta" || plan.AlreadyLinked[0].HarnessID != "two" {
		t.Errorf("AlreadyLinked = %v, want beta/two only", plan.AlreadyLinked)
	}
	if len(plan.AlreadyAbsent) != 1 || plan.AlreadyAbsent[0].Skill != "alpha" || plan.AlreadyAbsent[0].HarnessID != "two" {
		t.Errorf("AlreadyAbsent = %v, want alpha/two only", plan.AlreadyAbsent)
	}
}

func TestSkillManagedLinkMustPointAtItsOwnSource(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "alpha")
	beta := makeSkillDir(t, root, "beta")
	h := skillTestHarness("one", t.TempDir())
	// alpha's destination points at beta: not managed for alpha.
	symlink(t, beta, filepath.Join(h.SkillsDir, "alpha"))

	cell := skillCell(t, "alpha", h, root)
	plan, err := SkillReconcile([]SkillCell{cell}, map[string]bool{})
	if err != nil {
		t.Fatalf("SkillReconcile: %v", err)
	}
	if len(plan.Unmanaged) != 1 || len(plan.ToRemove) != 0 {
		t.Fatalf("plan = %+v, want alpha/one untouched", plan)
	}
}

func TestSkillLinksRefusesConflicts(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "alpha")
	h := skillTestHarness("one", t.TempDir())
	cell := skillCell(t, "alpha", h, root)

	t.Run("existing directory", func(t *testing.T) {
		if err := os.MkdirAll(cell.Target, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := SkillLinks([]SkillCell{cell})
		want := "refusing to replace existing path: " + cell.Target + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("SkillLinks error = %v, want %q", err, want)
		}
	})

	t.Run("unrelated symlink", func(t *testing.T) {
		h := skillTestHarness("two", t.TempDir())
		cell := skillCell(t, "alpha", h, root)
		symlink(t, "/elsewhere/target", cell.Target)
		_, err := SkillLinks([]SkillCell{cell})
		want := "refusing to replace unrelated symlink: " + cell.Target + "; no changes made"
		if err == nil || err.Error() != want {
			t.Fatalf("SkillLinks error = %v, want %q", err, want)
		}
	})
}

func TestSkillUnlinks(t *testing.T) {
	root := t.TempDir()
	alpha := makeSkillDir(t, root, "alpha")
	h := skillTestHarness("one", t.TempDir())
	managed := skillCell(t, "alpha", h, root)
	symlink(t, alpha, managed.Target)

	absentH := skillTestHarness("two", t.TempDir())
	absent := skillCell(t, "alpha", absentH, root)

	fileH := skillTestHarness("three", t.TempDir())
	file := skillCell(t, "alpha", fileH, root)
	writeFile(t, file.Target)

	linkH := skillTestHarness("four", t.TempDir())
	unrelated := skillCell(t, "alpha", linkH, root)
	symlink(t, "/elsewhere/target", unrelated.Target)

	plan, err := SkillUnlinks([]SkillCell{managed, absent, file, unrelated, managed})
	if err != nil {
		t.Fatalf("SkillUnlinks: %v", err)
	}
	if len(plan.ToRemove) != 1 || plan.ToRemove[0].HarnessID != "one" {
		t.Errorf("ToRemove = %v", plan.ToRemove)
	}
	if len(plan.AlreadyAbsent) != 1 || plan.AlreadyAbsent[0].HarnessID != "two" {
		t.Errorf("AlreadyAbsent = %v", plan.AlreadyAbsent)
	}
	if len(plan.Unmanaged) != 2 {
		t.Errorf("Unmanaged = %v, want two", plan.Unmanaged)
	}

	t.Run("missing sources do not prevent cleanup", func(t *testing.T) {
		h := skillTestHarness("gone", t.TempDir())
		cell := skillCell(t, "gone-skill", h, root)
		symlink(t, cell.Source, cell.Target)
		plan, err := SkillUnlinks([]SkillCell{cell})
		if err != nil {
			t.Fatalf("SkillUnlinks: %v", err)
		}
		if len(plan.ToRemove) != 1 {
			t.Fatalf("ToRemove = %v, want one", plan.ToRemove)
		}
	})
}

func TestSkillReconcileKeepsTargetClaimedBySelection(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "alpha")
	shared := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	one := skillTestHarness("one", shared)
	two := skillTestHarness("two", shared)
	symlink(t, filepath.Join(root, "alpha"), filepath.Join(shared, "alpha"))

	cells := []SkillCell{
		skillCell(t, "alpha", one, root),
		skillCell(t, "alpha", two, root),
	}
	plan, err := SkillReconcile(cells, map[string]bool{CellKey("alpha", "one"): true})
	if err != nil {
		t.Fatalf("SkillReconcile: %v", err)
	}
	if len(plan.ToRemove) != 0 {
		t.Errorf("ToRemove = %v, want none while a selected alias claims the target", plan.ToRemove)
	}
}

func TestSkillReconcileRejectsAmbiguousSources(t *testing.T) {
	root := t.TempDir()
	one := makeSkillDir(t, root, "one")
	two := makeSkillDir(t, root, "two")
	h := skillTestHarness("shared", t.TempDir())
	target := filepath.Join(h.SkillsDir, "alias")

	cells := []SkillCell{
		{Skill: "one", HarnessID: "shared", Source: one, Target: target},
		{Skill: "two", HarnessID: "shared", Source: two, Target: target},
	}
	_, err := SkillReconcile(cells, map[string]bool{CellKey("one", "shared"): true, CellKey("two", "shared"): true})
	want := "refusing to point one destination at different skills: " + target + "; no changes made"
	if err == nil || err.Error() != want {
		t.Fatalf("SkillReconcile error = %v, want %q", err, want)
	}
}

func TestSkillAvailability(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "synced")
	h := skillTestHarness("claude", t.TempDir())
	cell := skillCell(t, "synced", h, root)
	cell.Unavailable = "reserved"

	plan, err := SkillLinks([]SkillCell{cell})
	if err != nil {
		t.Fatalf("SkillLinks: %v", err)
	}
	if len(plan.ToCreate) != 0 || len(plan.Unavailable) != 1 {
		t.Fatalf("plan = %+v, want the pair unavailable", plan)
	}

	t.Run("an existing managed reserved link can be cleaned up", func(t *testing.T) {
		symlink(t, cell.Source, cell.Target)
		plan, err := SkillReconcile([]SkillCell{cell}, map[string]bool{})
		if err != nil {
			t.Fatalf("SkillReconcile: %v", err)
		}
		if len(plan.ToRemove) != 1 {
			t.Fatalf("ToRemove = %v, want the managed reserved link", plan.ToRemove)
		}
	})

	t.Run("a selected managed reserved link stays linked", func(t *testing.T) {
		plan, err := SkillReconcile([]SkillCell{cell}, map[string]bool{cell.Key(): true})
		if err != nil {
			t.Fatalf("SkillReconcile: %v", err)
		}
		if len(plan.AlreadyLinked) != 1 || len(plan.Unavailable) != 0 {
			t.Fatalf("plan = %+v, want already linked", plan)
		}
	})
}

func TestCreateSkillLinks(t *testing.T) {
	root := t.TempDir()
	alpha := makeSkillDir(t, root, "alpha")
	h := skillTestHarness("one", t.TempDir())
	cell := skillCell(t, "alpha", h, root)

	t.Run("creates missing directories and links", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		ctx := testContext(t)
		if err := CreateSkillLinks([]SkillCell{cell}, ctx, &stdout, &stderr); err != nil {
			t.Fatalf("CreateSkillLinks: %v", err)
		}
		if !strings.Contains(stdout.String(), "created symlink "+cell.Target+"\n") {
			t.Errorf("stdout = %q, want a created symlink line", stdout.String())
		}
		dest, err := os.Readlink(cell.Target)
		if err != nil || dest != alpha {
			t.Errorf("readlink = %q, %v; want %q", dest, err, alpha)
		}
	})

	t.Run("destination changing after planning is refused", func(t *testing.T) {
		h := skillTestHarness("two", t.TempDir())
		cell := skillCell(t, "alpha", h, root)
		writeFile(t, cell.Target)
		var stdout, stderr bytes.Buffer
		err := CreateSkillLinks([]SkillCell{cell}, testContext(t), &stdout, &stderr)
		want := "destination changed after planning; refusing to overwrite: " + cell.Target
		if err == nil || err.Error() != want {
			t.Fatalf("CreateSkillLinks error = %v, want %q", err, want)
		}
	})
}

func TestApplySkillsCreatesBeforeRemoving(t *testing.T) {
	root := t.TempDir()
	makeSkillDir(t, root, "alpha")
	makeSkillDir(t, root, "beta")
	ctx := testContext(t)

	removeH := skillTestHarness("remove", t.TempDir())
	remove := skillCell(t, "beta", removeH, root)
	symlink(t, remove.Source, remove.Target)

	createH := skillTestHarness("create", t.TempDir())
	create := skillCell(t, "alpha", createH, root)
	writeFile(t, create.Target) // appears after planning

	p := SkillsPlan{
		ToCreate: []SkillCell{create},
		ToRemove: []SkillCell{remove},
	}
	var stdout, stderr bytes.Buffer
	err := ApplySkills(p, ctx, &stdout, &stderr)
	if err == nil {
		t.Fatal("ApplySkills unexpectedly succeeded")
	}
	if _, statErr := os.Lstat(remove.Target); statErr != nil {
		t.Error("the managed link must survive a failed creation")
	}
	if !strings.Contains(stderr.String(), "No planned managed links were removed because link creation did not finish.\n") {
		t.Errorf("stderr = %q, want the removal guard", stderr.String())
	}

	t.Run("success applies both sides", func(t *testing.T) {
		h := skillTestHarness("ok", t.TempDir())
		create := skillCell(t, "alpha", h, root)
		p := SkillsPlan{ToCreate: []SkillCell{create}, ToRemove: []SkillCell{remove}}
		var stdout, stderr bytes.Buffer
		if err := ApplySkills(p, ctx, &stdout, &stderr); err != nil {
			t.Fatalf("ApplySkills: %v", err)
		}
		if _, err := os.Lstat(remove.Target); err == nil {
			t.Error("the managed link should have been removed")
		}
		if dest, err := os.Readlink(create.Target); err != nil || dest != create.Source {
			t.Errorf("readlink = %q, %v", dest, err)
		}
	})
}

func TestRemoveManagedSkillLinksStopsOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission checks do not apply")
	}
	root := t.TempDir()
	makeSkillDir(t, root, "alpha")
	ctx := testContext(t)
	dir := filepath.Join(ctx.Home, ".claude", "skills")
	h := skillTestHarness("claude", dir)
	cell := skillCell(t, "alpha", h, root)
	symlink(t, cell.Source, cell.Target)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	var stdout, stderr bytes.Buffer
	err := RemoveManagedSkillLinks([]SkillCell{cell}, ctx, &stdout, &stderr)
	if err == nil {
		t.Fatal("RemoveManagedSkillLinks unexpectedly succeeded")
	}
	wantStop := "stopped at " + cell.Target + "; earlier reported removals remain, and later destinations were not attempted.\n"
	if stderr.String() != wantStop {
		t.Errorf("stderr = %q, want %q", stderr.String(), wantStop)
	}
}

func TestRenderSkillsPlans(t *testing.T) {
	ctx := testContext(t)
	root := ctx.SourceSkillsDir
	h := skillTestHarness("one", filepath.Join(ctx.Home, ".agents", "skills"))
	cell := skillCell(t, "alpha", h, root)

	linkText := RenderSkillLinkPlan(SkillsPlan{ToCreate: []SkillCell{cell}}, ctx)
	for _, want := range []string{"Link plan:", "~/.agents/skills/alpha -> ~/.global-agents/skills/alpha"} {
		if !strings.Contains(linkText, want) {
			t.Errorf("link plan missing %q:\n%s", want, linkText)
		}
	}

	unlinkText := RenderSkillUnlinkPlan(SkillsPlan{ToRemove: []SkillCell{cell}}, ctx)
	if !strings.Contains(unlinkText, "Remove only these managed symlink entries") {
		t.Errorf("unlink plan missing the removal header:\n%s", unlinkText)
	}

	reconcileText := RenderSkillsReconcilePlan(SkillsPlan{
		ToCreate:      []SkillCell{cell},
		AlreadyLinked: []SkillCell{cell},
		AlreadyAbsent: []SkillCell{cell},
	}, ctx)
	for _, want := range []string{"Keep 1 already-linked and 1 absent destination(s) unchanged."} {
		if !strings.Contains(reconcileText, want) {
			t.Errorf("reconcile plan missing %q:\n%s", want, reconcileText)
		}
	}
}
