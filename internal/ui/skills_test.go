package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/skill"
)

func testMatrix(t *testing.T) (*SkillsMatrix, harness.Context) {
	t.Helper()
	ctx := testContext(t)
	harnesses := testHarnesses(ctx)
	m := &SkillsMatrix{
		Skills:    []string{"alpha", "beta"},
		Harnesses: harnesses,
		Cells:     make(map[string]MatrixCell),
		Initial:   make(map[string]bool),
	}
	for _, skill := range m.Skills {
		for _, h := range harnesses {
			cell := MatrixCell{
				Skill:       skill,
				HarnessID:   h.ID,
				HarnessName: h.Name,
				Source:      ctx.SourceSkillsDir + "/" + skill,
				Target:      h.SkillsDir + "/" + skill,
				State:       harness.Absent,
			}
			m.Cells[cell.Key()] = cell
		}
	}
	return m, ctx
}

func skillsReview(record *map[string]bool) func(map[string]bool) (Review, bool, error) {
	return func(selection map[string]bool) (Review, bool, error) {
		if record != nil {
			*record = selection
		}
		return Review{
			Text:    "Apply plan:\n  Create these symlinks",
			Summary: "Confirmation received; applying plan (create 1 symlink(s)).",
			Apply:   func() error { return nil },
		}, true, nil
	}
}

func newMatrixWizard(t *testing.T, matrix *SkillsMatrix, ctx harness.Context) *wizard {
	t.Helper()
	return newWizard(WizardFlow{
		Ctx:          ctx,
		Mode:         ModeSkills,
		Skills:       matrix,
		SkillsReview: skillsReview(nil),
	})
}

func keyRight() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyRight} }
func keyPgDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyPgDown} }

func TestLaunchModeMenu(t *testing.T) {
	ctx := testContext(t)

	t.Run("instructions choice", func(t *testing.T) {
		m := newWizard(WizardFlow{
			Harnesses: testHarnesses(ctx),
			Ctx:       ctx,
			Launch:    true,
			Review:    applyReview(nil),
		})
		if m.step != stepMode {
			t.Fatalf("step = %v, want mode", m.step)
		}
		view := viewText(m)
		for _, want := range []string{"What do you want to manage?", "Manage global instructions (AGENTS.md)", "Manage global skills (SKILL.md)", "Cancel"} {
			if !strings.Contains(flatText(m), want) && !strings.Contains(view, want) {
				t.Errorf("mode view missing %q:\n%s", want, view)
			}
		}
		if _, cmd := m.Update(keyRune('1')); cmd != nil {
			t.Fatal("choosing instructions should not quit")
		}
		if m.step != stepSelect || m.mode != ModeInstructions || !m.modeChosen {
			t.Fatalf("step = %v, mode = %v, chosen = %v", m.step, m.mode, m.modeChosen)
		}
	})

	t.Run("skills choice with a matrix", func(t *testing.T) {
		matrix, _ := testMatrix(t)
		m := newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: matrix, SkillsReview: skillsReview(nil)})
		if _, cmd := m.Update(keyRune('2')); cmd != nil {
			t.Fatal("choosing skills should not quit")
		}
		if m.step != stepMatrix || m.mode != ModeSkills || !m.modeChosen {
			t.Fatalf("step = %v, mode = %v, chosen = %v", m.step, m.mode, m.modeChosen)
		}
	})

	t.Run("skills choice without skills opens the recoverable empty matrix", func(t *testing.T) {
		m := newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: &SkillsMatrix{}})
		if _, cmd := m.Update(keyRune('2')); cmd != nil {
			t.Fatal("choosing an empty skills mode should not quit")
		}
		if m.step != stepMatrix || !m.modeChosen || m.mode != ModeSkills {
			t.Fatalf("step = %v, mode = %v, chosen = %v", m.step, m.mode, m.modeChosen)
		}
		if !strings.Contains(viewText(m), "(no shared skills found)") {
			t.Errorf("empty matrix view is missing its placeholder:\n%s", viewText(m))
		}
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		result := m.result()
		if !result.NoChanges || !result.ModeChosen || result.Mode != ModeSkills {
			t.Fatalf("result = %+v, want skills no-changes", result)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		m := newWizard(WizardFlow{Ctx: ctx, Launch: true})
		_, cmd := m.Update(keyEsc())
		assertQuits(t, cmd)
		if result := m.result(); !result.Cancelled || result.ModeChosen {
			t.Fatalf("result = %+v, want cancelled before a mode choice", result)
		}
	})

	t.Run("a broken instructions source does not block skills", func(t *testing.T) {
		want := os.ErrPermission
		matrix, _ := testMatrix(t)
		m := newWizard(WizardFlow{Ctx: ctx, Launch: true, InstructionsError: want, Skills: matrix, SkillsReview: skillsReview(nil)})
		if _, cmd := m.Update(keyRune('2')); cmd != nil {
			t.Fatal("choosing the healthy mode should not quit")
		}
		if m.step != stepMatrix || m.err != nil {
			t.Fatalf("step = %v, err = %v; want the skills matrix", m.step, m.err)
		}
	})

	t.Run("a broken skills source does not block instructions", func(t *testing.T) {
		want := os.ErrPermission
		m := newWizard(WizardFlow{
			Ctx:         ctx,
			Launch:      true,
			Harnesses:   testHarnesses(ctx),
			SkillsError: want,
			Review:      applyReview(nil),
		})
		if _, cmd := m.Update(keyRune('1')); cmd != nil {
			t.Fatal("choosing the healthy mode should not quit")
		}
		if m.step != stepSelect || m.err != nil {
			t.Fatalf("step = %v, err = %v; want the instructions picker", m.step, m.err)
		}
	})

	t.Run("choosing the broken mode shows a recoverable error", func(t *testing.T) {
		want := os.ErrPermission
		m := newWizard(WizardFlow{Ctx: ctx, Launch: true, SkillsError: want})
		if _, cmd := m.Update(keyRune('2')); cmd != nil {
			t.Fatal("a deferred preparation error should not quit")
		}
		if m.step != stepError || m.blocked != want || m.err != nil {
			t.Fatalf("step = %v, blocked = %v, err = %v; want the error screen", m.step, m.blocked, m.err)
		}
		if !strings.Contains(viewText(m), "Cannot prepare that mode.") {
			t.Errorf("error screen missing its subtitle:\n%s", viewText(m))
		}
		_, cmd := m.Update(keyCtrlC())
		assertQuits(t, cmd)
		if result := m.result(); !result.Cancelled || result.Applied || result.NoChanges {
			t.Fatalf("result = %+v, want cancelled only", result)
		}
	})
}

func TestMatrixToggleIndependent(t *testing.T) {
	matrix, ctx := testMatrix(t)
	m := newMatrixWizard(t, matrix, ctx)

	m.Update(keySpace()) // alpha/alpha
	m.Update(keyRight())
	m.Update(keySpace()) // alpha/beta only
	if got := m.MatrixSelection(); len(got) != 2 || !got[skill.CellKey("alpha", "alpha")] || !got[skill.CellKey("alpha", "beta")] {
		t.Fatalf("selection = %v, want alpha/alpha and alpha/beta", got)
	}
	if m.MatrixSelection()[skill.CellKey("beta", "alpha")] {
		t.Fatal("toggling one cell selected another")
	}

	m.Update(keyDown())
	m.Update(keySpace()) // beta/beta
	if got := m.MatrixSelection(); !got[skill.CellKey("beta", "beta")] || got[skill.CellKey("beta", "alpha")] {
		t.Fatalf("selection = %v, want beta/beta only on the second row", got)
	}
	if !strings.Contains(viewText(m), "✓") {
		t.Errorf("view does not show selected markers:\n%s", viewText(m))
	}
}

func TestMatrixSelectAllSkipsConflictsAndUnavailable(t *testing.T) {
	matrix, ctx := testMatrix(t)
	alpha := matrix.Cells[skill.CellKey("alpha", "alpha")]
	alpha.State = harness.Path
	matrix.Cells[alpha.Key()] = alpha
	beta := matrix.Cells[skill.CellKey("beta", "beta")]
	beta.Unavailable = "reserved"
	matrix.Cells[beta.Key()] = beta

	m := newMatrixWizard(t, matrix, ctx)
	m.Update(keyRune('a'))
	selection := m.MatrixSelection()
	if selection[alpha.Key()] || selection[beta.Key()] {
		t.Fatalf("selection = %v, conflicts and unavailable cells must be skipped", selection)
	}
	if !selection[skill.CellKey("alpha", "beta")] || !selection[skill.CellKey("beta", "alpha")] {
		t.Fatalf("selection = %v, want the four selectable cells", selection)
	}

	m.Update(keyRune('n'))
	if len(m.MatrixSelection()) != 0 {
		t.Fatal("n did not clear the matrix")
	}
}

func TestMatrixReservedManagedCellCanBeDeselected(t *testing.T) {
	matrix, ctx := testMatrix(t)
	cell := matrix.Cells[skill.CellKey("alpha", "alpha")]
	cell.State = harness.Managed
	cell.Unavailable = "reserved"
	matrix.Cells[cell.Key()] = cell
	matrix.Initial[cell.Key()] = true

	m := newMatrixWizard(t, matrix, ctx)
	if !m.MatrixSelection()[cell.Key()] {
		t.Fatal("managed reserved cell should start selected for cleanup")
	}
	m.Update(keySpace())
	if m.MatrixSelection()[cell.Key()] {
		t.Fatal("managed reserved cell could not be deselected")
	}
}

func TestMatrixScrollingReachesEveryCell(t *testing.T) {
	ctx := testContext(t)
	harnesses := testHarnesses(ctx)
	matrix := &SkillsMatrix{Harnesses: harnesses, Cells: make(map[string]MatrixCell), Initial: make(map[string]bool)}
	for i := 0; i < 30; i++ {
		skill := fmt.Sprintf("skill-%02d", i)
		matrix.Skills = append(matrix.Skills, skill)
		for _, h := range harnesses {
			cell := MatrixCell{Skill: skill, HarnessID: h.ID, HarnessName: h.Name, Target: h.SkillsDir + "/" + skill, State: harness.Absent}
			matrix.Cells[cell.Key()] = cell
		}
	}

	m := newMatrixWizard(t, matrix, ctx)
	m.width = 50
	m.height = 16
	for i := 0; i < 25; i++ {
		m.Update(keyDown())
	}
	if !strings.Contains(viewText(m), "skill-25") {
		t.Fatalf("scrolled view does not show a later skill:\n%s", viewText(m))
	}
	for i := 0; i < 2; i++ {
		m.Update(keyRight())
	}
	if !strings.Contains(flatText(m), "Gamma") {
		t.Fatalf("horizontally scrolled view does not focus the last harness:\n%s", viewText(m))
	}
	if !strings.Contains(flatText(m), "skill-25 Gamma") {
		t.Fatalf("details line missing the focused cell:\n%s", viewText(m))
	}

	// Every row and column pair remains reachable.
	for row := 0; row < len(matrix.Skills); row++ {
		m.matrixRow = row
		for col := 0; col < len(matrix.Harnesses); col++ {
			m.matrixCol = col
			if !strings.Contains(viewText(m), matrix.Skills[row]) {
				t.Fatalf("row %d is not visible at col %d", row, col)
			}
		}
	}
}

func TestMatrixReviewAndBack(t *testing.T) {
	matrix, ctx := testMatrix(t)
	var reviewed map[string]bool
	m := newWizard(WizardFlow{Ctx: ctx, Mode: ModeSkills, Skills: matrix, SkillsReview: skillsReview(&reviewed)})

	m.Update(keySpace())
	_, cmd := m.Update(keyEnter())
	if cmd != nil {
		t.Fatal("entering review should not quit")
	}
	if m.step != stepReview {
		t.Fatalf("step = %v, want review", m.step)
	}
	if len(reviewed) != 1 || !reviewed[skill.CellKey("alpha", "alpha")] {
		t.Fatalf("reviewed = %v", reviewed)
	}

	_, cmd = m.Update(keyEsc())
	if cmd != nil || m.cancelled || m.step != stepMatrix {
		t.Fatalf("esc should return to the matrix; cmd = %v, step = %v", cmd != nil, m.step)
	}
	if !m.MatrixSelection()[skill.CellKey("alpha", "alpha")] {
		t.Fatal("selection was lost on the way back")
	}

	_, cmd = m.Update(keyEsc())
	assertQuits(t, cmd)
	if result := m.result(); !result.Cancelled {
		t.Fatalf("result = %+v, want cancelled", result)
	}
}

func TestLaunchMatrixEscReturnsToMode(t *testing.T) {
	matrix, ctx := testMatrix(t)
	m := newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: matrix, SkillsReview: skillsReview(nil)})
	m.Update(keyRune('2'))
	m.Update(keyDown())
	m.Update(keyRight())
	m.Update(keySpace()) // beta/beta

	_, cmd := m.Update(keyEsc())
	if cmd != nil || m.cancelled {
		t.Fatalf("esc at launch matrix should return to the mode menu; cancelled = %v", m.cancelled)
	}
	if m.step != stepMode || m.modeChosen || m.blocked != nil {
		t.Fatalf("step = %v, modeChosen = %v, blocked = %v", m.step, m.modeChosen, m.blocked)
	}
	if m.matrixRow != 1 || m.matrixCol != 1 {
		t.Errorf("matrix cursor = (%d, %d), want (1, 1)", m.matrixRow, m.matrixCol)
	}
	if !m.MatrixSelection()[skill.CellKey("beta", "beta")] {
		t.Error("matrix selection was lost on the way back")
	}

	m.Update(keyRune('2'))
	if m.step != stepMatrix || m.matrixRow != 1 || m.matrixCol != 1 {
		t.Fatalf("re-entered matrix step = %v, cursor = (%d, %d)", m.step, m.matrixRow, m.matrixCol)
	}
}

func TestReviewScrolling(t *testing.T) {
	matrix, ctx := testMatrix(t)
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	m := newWizard(WizardFlow{
		Ctx:    ctx,
		Mode:   ModeSkills,
		Skills: matrix,
		SkillsReview: func(map[string]bool) (Review, bool, error) {
			return Review{Text: strings.Join(lines, "\n"), Summary: "applied", Apply: func() error { return nil }}, true, nil
		},
	})
	m.Update(keySpace())
	m.Update(keyEnter())
	m.width = 60
	m.height = 18

	view := viewText(m)
	if !strings.Contains(view, "line 00") {
		t.Fatalf("review does not start at the first line:\n%s", view)
	}
	if !strings.Contains(view, "lines 1-") {
		t.Errorf("review is missing the scroll indicator:\n%s", view)
	}

	m.Update(keyPgDown())
	for i := 0; i < 14; i++ {
		m.Update(keyPgDown())
	}
	view = viewText(m)
	if !strings.Contains(view, "line 30") && !strings.Contains(view, "line 31") {
		t.Errorf("pgdown did not scroll the review:\n%s", view)
	}
	if !strings.Contains(view, "Apply this plan?") {
		t.Errorf("review lost the apply prompt while scrolling:\n%s", view)
	}
}

func TestReviewLinesCacheInvalidates(t *testing.T) {
	matrix, ctx := testMatrix(t)
	m := newWizard(WizardFlow{
		Ctx:    ctx,
		Mode:   ModeSkills,
		Skills: matrix,
		SkillsReview: func(map[string]bool) (Review, bool, error) {
			return Review{Text: "first plan line\nsecond plan line", Summary: "applied", Apply: func() error { return nil }}, true, nil
		},
	})
	m.Update(keySpace())
	m.Update(keyEnter())
	if m.step != stepReview {
		t.Fatalf("step = %v, want review", m.step)
	}

	first := m.reviewLines()
	second := m.reviewLines()
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("review lines are empty")
	}
	if &first[0] != &second[0] {
		t.Error("an unchanged review re-wrapped instead of using the cache")
	}

	m.width += 10
	_ = m.reviewLines()
	if m.reviewLinesWidth != m.width {
		t.Error("a resize did not refresh the review-line cache")
	}
	m.dark = !m.dark
	_ = m.reviewLines()
	if m.reviewLinesDark != m.dark {
		t.Error("a theme change did not refresh the review-line cache")
	}

	m.review.Text = "a different plan"
	got := strings.Join(m.reviewLines(), "\n")
	if !strings.Contains(got, "a different plan") {
		t.Errorf("a new review was not rendered:\n%s", got)
	}
	if &first[0] == &m.reviewLines()[0] {
		t.Error("a new review reused the old cache")
	}
}

func TestReviewScrollClamps(t *testing.T) {
	matrix, ctx := testMatrix(t)
	m := newWizard(WizardFlow{
		Ctx:    ctx,
		Mode:   ModeSkills,
		Skills: matrix,
		SkillsReview: func(map[string]bool) (Review, bool, error) {
			return Review{Text: "only one line", Summary: "applied", Apply: func() error { return nil }}, true, nil
		},
	})
	m.Update(keySpace())
	m.Update(keyEnter())
	m.Update(keyPgDown())
	if m.reviewOffset != 0 {
		t.Fatalf("reviewOffset = %d, want 0 for a short plan", m.reviewOffset)
	}
}

func TestMatrixHelp(t *testing.T) {
	matrix, ctx := testMatrix(t)
	m := newMatrixWizard(t, matrix, ctx)
	short := viewText(m)
	for _, want := range []string{"space toggle", "enter review", "esc cancel", "? more"} {
		if !strings.Contains(short, want) {
			t.Errorf("short help missing %q:\n%s", want, short)
		}
	}
	m.Update(keyRune('?'))
	full := viewText(m)
	for _, want := range []string{"select all", "select none", "↑/↓", "←/→"} {
		if !strings.Contains(full, want) {
			t.Errorf("full help missing %q:\n%s", want, full)
		}
	}
}

func TestSkillsTable(t *testing.T) {
	matrix, ctx := testMatrix(t)
	cell := matrix.Cells[skill.CellKey("alpha", "alpha")]
	cell.State = harness.Managed
	matrix.Cells[cell.Key()] = cell
	conflict := matrix.Cells[skill.CellKey("beta", "beta")]
	conflict.State = harness.Path
	matrix.Cells[conflict.Key()] = conflict
	unavailable := matrix.Cells[skill.CellKey("beta", "gamma")]
	unavailable.Unavailable = "reserved"
	matrix.Cells[unavailable.Key()] = unavailable

	table, err := RenderSkillsTable(*matrix, ctx, map[string]bool{cell.Key(): true})
	if err != nil {
		t.Fatalf("RenderSkillsTable: %v", err)
	}
	for _, want := range []string{
		"Shared skills: " + ctx.SourceSkillsDir,
		"alpha",
		"beta",
		"✓",
		"!",
		"–",
		"Legend: ✓ managed",
		"existing path; not overwritten",
		"reserved",
	} {
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q:\n%s", want, table)
		}
	}
}

func TestSkillsTableEmpty(t *testing.T) {
	ctx := testContext(t)
	table, err := RenderSkillsTable(SkillsMatrix{}, ctx, nil)
	if err != nil {
		t.Fatalf("RenderSkillsTable: %v", err)
	}
	if !strings.Contains(table, "(no shared skills)") || !strings.Contains(table, "Shared skills:") {
		t.Errorf("empty table = %q", table)
	}
}

func TestMatrixStatusCounts(t *testing.T) {
	matrix, ctx := testMatrix(t)
	managed := matrix.Cells[skill.CellKey("alpha", "alpha")]
	managed.State = harness.Managed
	matrix.Cells[managed.Key()] = managed
	unselected := matrix.Cells[skill.CellKey("beta", "beta")]
	unselected.State = harness.Managed
	matrix.Cells[unselected.Key()] = unselected

	m := newMatrixWizard(t, matrix, ctx)
	m.Update(keySpace()) // keep alpha/alpha
	m.Update(keyRight())
	m.Update(keyRight())
	m.Update(keySpace()) // create alpha/gamma
	status := m.matrixStatus()
	for _, want := range []string{"Selected 2 of 6", "create 1", "remove 1"} {
		if !strings.Contains(status, want) {
			t.Errorf("status = %q, missing %q", status, want)
		}
	}
}

func TestMatrixUsesWindowHeight(t *testing.T) {
	matrix, ctx := testMatrix(t)
	m := newMatrixWizard(t, matrix, ctx)
	m.Update(tea.WindowSizeMsg{Width: 82, Height: 20})
	if m.height != 20 {
		t.Fatalf("height = %d, want 20", m.height)
	}
}
