package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// The model tests drive Update with key messages and assert on the model
// state and View() content, without a terminal. Assertions run on the view
// with style sequences stripped so they read like the screen.

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func viewText(m interface{ View() tea.View }) string {
	return ansiPattern.ReplaceAllString(m.View().Content, "")
}

// flatText is the view without styles or line breaks, for assertions that
// must survive lipgloss word wrapping.
func flatText(m interface{ View() tea.View }) string {
	return strings.ReplaceAll(viewText(m), "\n", "")
}

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

// testHarnesses returns three harnesses whose targets all sit in ctx's home,
// so nothing is installed and nothing touches the real filesystem.
func testHarnesses(ctx harness.Context) []harness.Harness {
	return []harness.Harness{
		{ID: "alpha", Name: "Alpha", Target: filepath.Join(ctx.Home, "alpha", "AGENTS.md"), Executables: []string{"alpha"}},
		{ID: "beta", Name: "Beta", Target: filepath.Join(ctx.Home, "beta", "AGENTS.md"), Executables: []string{"beta"}},
		{ID: "gamma", Name: "Gamma", Target: filepath.Join(ctx.Home, "gamma", "AGENTS.md"), Executables: []string{"gamma"}},
	}
}

func keyUp() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: tea.KeyUp} }
func keyDown() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func keyLeft() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyLeft} }
func keyEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func keyEsc() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyEscape} }
func keySpace() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }
func keyCtrlC() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl} }
func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// assertQuits fails unless cmd exits the program with a QuitMsg.
func assertQuits(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no quit command returned")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", cmd())
	}
}

// assertNotQuit fails when cmd exits the program; other commands (the setup
// input's cursor blink) are expected and safe to run.
func assertNotQuit(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	if _, ok := cmd().(tea.QuitMsg); ok {
		t.Fatal("unexpected quit")
	}
}

// focusedName returns the harness on the wizard's cursor row.
func focusedName(t *testing.T, m *wizard) string {
	t.Helper()
	for _, line := range strings.Split(viewText(m), "\n") {
		if !strings.Contains(line, "❯") {
			continue
		}
		for _, h := range m.flow.Harnesses {
			if strings.Contains(line, h.Name) {
				return h.Name
			}
		}
	}
	t.Fatalf("no focused row in view:\n%s", viewText(m))
	return ""
}

// applyReview is a Review callback that records the selection and returns a
// canned review for it.
func applyReview(record *map[string]bool) func(map[string]bool) (Review, bool, error) {
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

func newSelectWizard(t *testing.T, ctx harness.Context) *wizard {
	t.Helper()
	return newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Review:    applyReview(nil),
	})
}

func TestWizardCursorMovement(t *testing.T) {
	ctx := testContext(t)
	m := newSelectWizard(t, ctx)

	if got := focusedName(t, m); got != "Alpha" {
		t.Fatalf("initial focus = %q, want Alpha", got)
	}

	m.Update(keyDown())
	if got := focusedName(t, m); got != "Beta" {
		t.Fatalf("after down = %q, want Beta", got)
	}
	m.Update(keyDown())
	m.Update(keyDown())
	if got := focusedName(t, m); got != "Alpha" {
		t.Fatalf("after wrapping down = %q, want Alpha", got)
	}
	m.Update(keyUp())
	if got := focusedName(t, m); got != "Gamma" {
		t.Fatalf("after wrapping up = %q, want Gamma", got)
	}
	m.Update(keyRune('j'))
	if got := focusedName(t, m); got != "Alpha" {
		t.Fatalf("after wrapping j = %q, want Alpha", got)
	}
	m.Update(keyRune('k'))
	if got := focusedName(t, m); got != "Gamma" {
		t.Fatalf("after wrapping k = %q, want Gamma", got)
	}
}

func TestWizardToggle(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Initial:   map[string]bool{"beta": true},
		Review:    applyReview(nil),
	})

	if !m.Selection()["beta"] {
		t.Fatal("initial selection lost beta")
	}
	m.Update(keySpace()) // alpha on
	m.Update(keyDown())
	m.Update(keySpace()) // beta off
	m.Update(keyDown())
	m.Update(keySpace()) // gamma on

	selection := m.Selection()
	if !selection["alpha"] || selection["beta"] || !selection["gamma"] {
		t.Fatalf("selection = %v, want alpha and gamma", selection)
	}
	view := viewText(m)
	if got := strings.Count(view, "✓"); got != 2 || !strings.Contains(view, "Alpha") || !strings.Contains(view, "Gamma") {
		t.Errorf("view does not reflect the selection:\n%s", view)
	}
}

func TestWizardSelectAllAndNone(t *testing.T) {
	ctx := testContext(t)
	m := newSelectWizard(t, ctx)

	m.Update(keyRune('a'))
	if selection := m.Selection(); len(selection) != 3 {
		t.Fatalf("after a: selection = %v, want all three", selection)
	}
	m.Update(keyRune('n'))
	if selection := m.Selection(); len(selection) != 0 {
		t.Fatalf("after n: selection = %v, want none", selection)
	}
	m.Update(keyRune('A'))
	if selection := m.Selection(); len(selection) != 3 {
		t.Fatalf("after A: selection = %v, want all three", selection)
	}
	m.Update(keyRune('N'))
	if selection := m.Selection(); len(selection) != 0 {
		t.Fatalf("after N: selection = %v, want none", selection)
	}
}

func TestWizardSelectionIsACopy(t *testing.T) {
	ctx := testContext(t)
	m := newSelectWizard(t, ctx)
	m.Update(keySpace())

	selection := m.Selection()
	selection["alpha"] = false
	if !m.Selection()["alpha"] {
		t.Fatal("mutating the returned selection changed the model")
	}
}

func TestWizardEnterReviewApply(t *testing.T) {
	ctx := testContext(t)
	var reviewed map[string]bool
	m := newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Review:    applyReview(&reviewed),
	})

	m.Update(keySpace())
	_, cmd := m.Update(keyEnter())
	if cmd != nil {
		t.Fatal("entering the review step should not quit the program")
	}
	if m.step != stepReview {
		t.Fatalf("step = %v, want review", m.step)
	}
	if len(reviewed) != 1 || !reviewed["alpha"] {
		t.Fatalf("reviewed selection = %v, want alpha only", reviewed)
	}
	view := viewText(m)
	for _, want := range []string{"Apply plan:", "Apply this plan?", "Review and confirm."} {
		if !strings.Contains(view, want) {
			t.Errorf("review view is missing %q:\n%s", want, view)
		}
	}

	_, cmd = m.Update(keyEnter())
	assertQuits(t, cmd)
	result := m.result()
	if !result.Applied || result.Cancelled || result.NoChanges {
		t.Fatalf("result = %+v, want applied only", result)
	}
	if result.Review.Summary == "" || result.Review.Apply == nil {
		t.Fatalf("result review = %+v, want summary and apply", result.Review)
	}
}

func TestWizardReviewEscReturnsToSelect(t *testing.T) {
	ctx := testContext(t)
	m := newSelectWizard(t, ctx)

	m.Update(keySpace())
	m.Update(keyEnter())
	if m.step != stepReview {
		t.Fatalf("step = %v, want review", m.step)
	}

	_, cmd := m.Update(keyEsc())
	if cmd != nil || m.cancelled {
		t.Fatalf("esc at review should return to select, cmd = %v, cancelled = %v", cmd != nil, m.cancelled)
	}
	if m.step != stepSelect {
		t.Fatalf("step = %v, want select", m.step)
	}
	if !strings.Contains(viewText(m), "Link your shared instructions into each harness.") {
		t.Errorf("select view not shown after esc:\n%s", viewText(m))
	}

	// The selection survived the round trip.
	if !m.Selection()["alpha"] {
		t.Fatal("selection was lost on the way back")
	}

	_, cmd = m.Update(keyEsc())
	assertQuits(t, cmd)
	if result := m.result(); !result.Cancelled {
		t.Fatalf("result = %+v, want cancelled", result)
	}
}

func TestWizardNoChanges(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Review: func(map[string]bool) (Review, bool, error) {
			return Review{}, false, nil
		},
	})

	_, cmd := m.Update(keyEnter())
	assertQuits(t, cmd)
	if result := m.result(); !result.NoChanges || result.Applied || result.Cancelled {
		t.Fatalf("result = %+v, want no-changes only", result)
	}
}

func TestWizardReviewError(t *testing.T) {
	ctx := testContext(t)
	want := os.ErrPermission
	m := newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Review: func(map[string]bool) (Review, bool, error) {
			return Review{}, false, want
		},
	})

	_, cmd := m.Update(keyEnter())
	assertQuits(t, cmd)
	if m.err != want {
		t.Fatalf("err = %v, want %v", m.err, want)
	}
}

func TestWizardCancelKeys(t *testing.T) {
	ctx := testContext(t)

	for _, tc := range []struct {
		name string
		keys []tea.KeyPressMsg
	}{
		{name: "esc at select", keys: []tea.KeyPressMsg{keyEsc()}},
		{name: "ctrl+c at select", keys: []tea.KeyPressMsg{keyCtrlC()}},
		{name: "ctrl+c at review", keys: []tea.KeyPressMsg{keySpace(), keyEnter(), keyCtrlC()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newSelectWizard(t, ctx)
			var cmd tea.Cmd
			for _, key := range tc.keys {
				_, cmd = m.Update(key)
			}
			assertQuits(t, cmd)
			if result := m.result(); !result.Cancelled || result.Applied || result.NoChanges {
				t.Fatalf("result = %+v, want cancelled only", result)
			}
		})
	}
}

func TestWizardViewAndHelp(t *testing.T) {
	ctx := testContext(t)
	m := newSelectWizard(t, ctx)
	view := m.View()

	if !view.AltScreen {
		t.Error("wizard view is not declared as an alternate screen view")
	}
	for _, want := range []string{"agent-links", "Harness", "Installed", "Global instructions", "Alpha"} {
		if !strings.Contains(viewText(m), want) {
			t.Errorf("view is missing %q:\n%s", want, viewText(m))
		}
	}
	if !strings.Contains(viewText(m), "Selected 0 of 3") {
		t.Errorf("view is missing the selection status:\n%s", viewText(m))
	}
	short := viewText(m)
	for _, want := range []string{"space toggle", "enter review", "esc cancel", "? more"} {
		if !strings.Contains(short, want) {
			t.Errorf("short help is missing %q:\n%s", want, short)
		}
	}

	m.Update(keyRune('?'))
	full := viewText(m)
	for _, want := range []string{"select all", "select none", "enter review"} {
		if !strings.Contains(full, want) {
			t.Errorf("full help is missing %q:\n%s", want, full)
		}
	}
}

func TestWizardAdaptsToBackgroundColor(t *testing.T) {
	ctx := testContext(t)
	m := newSelectWizard(t, ctx)

	m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}})
	if m.dark {
		t.Error("white background reported as dark")
	}
	m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}})
	if !m.dark {
		t.Error("dark background reported as light")
	}
}

func TestWizardSetupBlankThenSelect(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Setup:     true,
		Review:    applyReview(nil),
	})

	if m.step != stepSetup {
		t.Fatalf("step = %v, want setup", m.step)
	}
	view := viewText(m)
	for _, want := range []string{
		"No shared instructions file found at",
		"Copy an existing instructions file",
		"Create a blank AGENTS.md",
		"Cancel",
	} {
		if !strings.Contains(flatText(m), want) && !strings.Contains(view, want) {
			t.Errorf("setup view is missing %q:\n%s", want, view)
		}
	}

	// The 2 accelerator still works.
	if _, cmd := m.Update(keyRune('2')); cmd != nil {
		t.Fatal("creating a blank file should not quit the wizard")
	}
	if _, err := os.Stat(ctx.SourceFile); err != nil {
		t.Fatalf("blank source file not created: %v", err)
	}
	if m.step != stepSelect {
		t.Fatalf("step = %v, want select after setup", m.step)
	}
	if result := m.result(); !result.Created || len(result.Notices) != 1 || !strings.Contains(result.Notices[0], "Created blank instructions file: "+ctx.SourceFile) {
		t.Fatalf("result = %+v, want Created with the created notice", result)
	}
	if !strings.Contains(viewText(m), "Link your shared instructions into each harness.") {
		t.Errorf("select view not shown after setup:\n%s", viewText(m))
	}
}

func TestWizardSetupMenuNavigationAndCancel(t *testing.T) {
	ctx := testContext(t)

	t.Run("down then enter creates a blank file", func(t *testing.T) {
		m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
		m.Update(keyDown())
		if _, cmd := m.Update(keyEnter()); cmd != nil {
			t.Fatal("creating a blank file should not quit the wizard")
		}
		if _, err := os.Stat(ctx.SourceFile); err != nil {
			t.Fatalf("blank source file not created: %v", err)
		}
	})

	t.Run("esc cancels", func(t *testing.T) {
		m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
		_, cmd := m.Update(keyEsc())
		assertQuits(t, cmd)
		if result := m.result(); !result.Cancelled {
			t.Fatalf("result = %+v, want cancelled", result)
		}
	})

	t.Run("q cancels", func(t *testing.T) {
		m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
		_, cmd := m.Update(keyRune('q'))
		assertQuits(t, cmd)
		if result := m.result(); !result.Cancelled {
			t.Fatalf("result = %+v, want cancelled", result)
		}
	})

	t.Run("enter on Cancel cancels", func(t *testing.T) {
		m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
		m.Update(keyUp()) // wraps to Cancel
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if result := m.result(); !result.Cancelled {
			t.Fatalf("result = %+v, want cancelled", result)
		}
	})
}

func TestWizardSetupCopyPath(t *testing.T) {
	ctx := testContext(t)
	selected := filepath.Join(ctx.Home, "notes", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(selected, []byte("# Shared\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
	m.Update(keyRune('1'))
	if m.setup.step != setupCopyPath {
		t.Fatalf("setup step = %v, want copy path", m.setup.step)
	}
	if !strings.Contains(viewText(m), "Path to the existing instructions file") {
		t.Errorf("path prompt not shown:\n%s", viewText(m))
	}

	// Live validation: an invalid path shows the message and enter holds.
	missing := filepath.Join(ctx.Home, "missing.md")
	m.setup.input.SetValue(missing)
	if !strings.Contains(flatText(m), "not found: "+missing) {
		t.Errorf("validation message not shown:\n%s", viewText(m))
	}
	if _, cmd := m.Update(keyEnter()); cmd != nil {
		t.Fatal("an invalid path should not quit the program")
	}
	if _, err := os.Stat(ctx.SourceFile); err == nil {
		t.Fatal("source file should not exist after an invalid path")
	}

	// A valid path shows the green check and copies on enter.
	m.setup.input.SetValue(selected)
	if !strings.Contains(viewText(m), "✓ file exists") {
		t.Errorf("valid-path check not shown:\n%s", viewText(m))
	}
	if _, cmd := m.Update(keyEnter()); cmd != nil {
		t.Fatal("copying should not quit the wizard")
	}
	if m.step != stepSelect {
		t.Fatalf("step = %v, want select after copy", m.step)
	}
	result := m.result()
	want := "Created " + ctx.SourceFile + " from " + selected
	if !result.Created || len(result.Notices) != 1 || result.Notices[0] != want {
		t.Fatalf("result = %+v, want Created with notice %q", result, want)
	}
	contents, err := os.ReadFile(ctx.SourceFile)
	if err != nil || string(contents) != "# Shared\n" {
		t.Fatalf("source contents = %q, %v", contents, err)
	}

	t.Run("esc backs out to the menu", func(t *testing.T) {
		fresh := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
		fresh.Update(keyRune('1'))
		_, cmd := fresh.Update(keyEsc())
		if cmd != nil || fresh.cancelled {
			t.Fatalf("esc in the path step should return to the menu; cancelled = %v", fresh.cancelled)
		}
		if fresh.setup.step != setupMenu {
			t.Fatalf("setup step = %v, want menu", fresh.setup.step)
		}
	})

	t.Run("ctrl+c in the path step cancels", func(t *testing.T) {
		fresh := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Setup: true, Review: applyReview(nil)})
		fresh.Update(keyRune('1'))
		_, cmd := fresh.Update(keyCtrlC())
		assertQuits(t, cmd)
		if result := fresh.result(); !result.Cancelled {
			t.Fatalf("result = %+v, want cancelled", result)
		}
	})
}

func newLaunchSelectWizard(t *testing.T, ctx harness.Context, initial map[string]bool) *wizard {
	t.Helper()
	return newWizard(WizardFlow{
		Harnesses: testHarnesses(ctx),
		Ctx:       ctx,
		Initial:   initial,
		Launch:    true,
		Review:    applyReview(nil),
	})
}

func TestLaunchSelectEscReturnsToMode(t *testing.T) {
	ctx := testContext(t)
	m := newLaunchSelectWizard(t, ctx, map[string]bool{"beta": true})
	m.Update(keyRune('1'))
	if m.step != stepSelect || !m.modeChosen {
		t.Fatalf("step = %v, modeChosen = %v; want the picker", m.step, m.modeChosen)
	}

	m.Update(keyDown())
	m.Update(keyDown())
	m.Update(keySpace()) // gamma on
	_, cmd := m.Update(keyEsc())
	if cmd != nil || m.cancelled {
		t.Fatalf("esc at launch select should return to the mode menu; cancelled = %v", m.cancelled)
	}
	if m.step != stepMode || m.modeChosen || m.blocked != nil {
		t.Fatalf("step = %v, modeChosen = %v, blocked = %v", m.step, m.modeChosen, m.blocked)
	}
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2", m.cursor)
	}
	selection := m.Selection()
	if !selection["beta"] || !selection["gamma"] {
		t.Errorf("selection = %v, want beta and gamma", selection)
	}
}

func TestLaunchReviewEscBacksThroughSelectToMode(t *testing.T) {
	ctx := testContext(t)
	m := newLaunchSelectWizard(t, ctx, nil)
	m.Update(keyRune('1'))
	m.Update(keySpace())
	m.Update(keyEnter())
	if m.step != stepReview {
		t.Fatalf("step = %v, want review", m.step)
	}

	m.Update(keyEsc())
	if m.step != stepSelect || !m.Selection()["alpha"] {
		t.Fatalf("step = %v, selection = %v; want select with alpha", m.step, m.Selection())
	}
	m.Update(keyEsc())
	if m.step != stepMode || m.modeChosen {
		t.Fatalf("step = %v, modeChosen = %v; want the mode menu", m.step, m.modeChosen)
	}
	if !m.Selection()["alpha"] {
		t.Error("selection was lost after returning to the mode menu")
	}
}

func TestLaunchSetupMenuEscReturnsToMode(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Launch: true, Setup: true, Review: applyReview(nil)})
	m.Update(keyRune('1'))
	if m.step != stepSetup {
		t.Fatalf("step = %v, want setup", m.step)
	}

	_, cmd := m.Update(keyEsc())
	if cmd != nil || m.cancelled {
		t.Fatalf("esc at the setup menu should return to the mode menu; cancelled = %v", m.cancelled)
	}
	if m.step != stepMode || m.modeChosen {
		t.Fatalf("step = %v, modeChosen = %v", m.step, m.modeChosen)
	}

	// The path step still backs out to the setup menu, not to the mode menu.
	m.Update(keyRune('1'))
	_, cmd = m.Update(keyRune('1'))
	assertNotQuit(t, cmd)
	if m.setup.step != setupCopyPath {
		t.Fatalf("setup step = %v, want copy path", m.setup.step)
	}
	m.Update(keyEsc())
	if m.step != stepSetup || m.setup.step != setupMenu {
		t.Fatalf("step = %v, setup step = %v; want the setup menu", m.step, m.setup.step)
	}
}

func TestLaunchSetupCreationSkipsSetupOnReentry(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{Harnesses: testHarnesses(ctx), Ctx: ctx, Launch: true, Setup: true, Review: applyReview(nil)})
	m.Update(keyRune('1'))
	m.Update(keyRune('2'))
	if _, err := os.Stat(ctx.SourceFile); err != nil {
		t.Fatalf("blank source file not created: %v", err)
	}
	if m.flow.Setup {
		t.Error("flow.Setup still true after creating the file")
	}
	if m.step != stepSelect {
		t.Fatalf("step = %v, want select", m.step)
	}

	m.Update(keyEsc())
	if m.step != stepMode {
		t.Fatalf("step = %v, want mode", m.step)
	}
	m.Update(keyRune('1'))
	if m.step != stepSelect {
		t.Fatalf("step = %v, want select directly (setup must be skipped)", m.step)
	}
	if strings.Contains(viewText(m), "No shared instructions file found at") {
		t.Errorf("setup screen was shown again:\n%s", viewText(m))
	}
}

func TestLaunchBrokenModesAreRecoverable(t *testing.T) {
	ctx := testContext(t)
	want := os.ErrPermission
	matrix, _ := testMatrix(t)

	t.Run("broken instructions", func(t *testing.T) {
		m := newWizard(WizardFlow{
			Ctx:               ctx,
			Launch:            true,
			InstructionsError: want,
			Skills:            matrix,
			SkillsReview:      skillsReview(nil),
		})
		m.Update(keyRune('1'))
		if m.step != stepError || m.blocked != want || m.err != nil {
			t.Fatalf("step = %v, blocked = %v, err = %v", m.step, m.blocked, m.err)
		}
		m.Update(keyEsc())
		if m.step != stepMode || m.blocked != nil || m.modeChosen {
			t.Fatalf("step = %v, blocked = %v, modeChosen = %v", m.step, m.blocked, m.modeChosen)
		}
		m.Update(keyRune('2'))
		if m.step != stepMatrix || m.err != nil {
			t.Fatalf("step = %v, err = %v; want the healthy skills matrix", m.step, m.err)
		}
	})

	t.Run("broken skills", func(t *testing.T) {
		m := newWizard(WizardFlow{
			Ctx:         ctx,
			Launch:      true,
			Harnesses:   testHarnesses(ctx),
			SkillsError: want,
			Review:      applyReview(nil),
		})
		m.Update(keyRune('2'))
		if m.step != stepError || m.blocked != want || m.err != nil {
			t.Fatalf("step = %v, blocked = %v, err = %v", m.step, m.blocked, m.err)
		}
		m.Update(keyEsc())
		m.Update(keyRune('1'))
		if m.step != stepSelect || m.err != nil {
			t.Fatalf("step = %v, err = %v; want the healthy picker", m.step, m.err)
		}
	})
}

func TestLaunchCtrlCCancelsEveryStep(t *testing.T) {
	ctx := testContext(t)
	matrix, _ := testMatrix(t)
	want := os.ErrPermission

	cases := []struct {
		name  string
		model *wizard
		keys  []tea.KeyPressMsg
	}{
		{name: "mode", model: newWizard(WizardFlow{Ctx: ctx, Launch: true})},
		{name: "setup menu", model: newWizard(WizardFlow{Ctx: ctx, Launch: true, Setup: true}), keys: []tea.KeyPressMsg{keyRune('1')}},
		{name: "setup path", model: newWizard(WizardFlow{Ctx: ctx, Launch: true, Setup: true}), keys: []tea.KeyPressMsg{keyRune('1'), keyRune('1')}},
		{name: "select", model: newLaunchSelectWizard(t, ctx, nil), keys: []tea.KeyPressMsg{keyRune('1')}},
		{name: "matrix", model: newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: matrix, SkillsReview: skillsReview(nil)}), keys: []tea.KeyPressMsg{keyRune('2')}},
		{name: "review", model: newLaunchSelectWizard(t, ctx, nil), keys: []tea.KeyPressMsg{keyRune('1'), keySpace(), keyEnter()}},
		{name: "error", model: newWizard(WizardFlow{Ctx: ctx, Launch: true, SkillsError: want}), keys: []tea.KeyPressMsg{keyRune('2')}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range tc.keys {
				_, cmd := tc.model.Update(key)
				assertNotQuit(t, cmd)
			}
			_, cmd := tc.model.Update(keyCtrlC())
			assertQuits(t, cmd)
			if result := tc.model.result(); !result.Cancelled || result.Applied || result.NoChanges {
				t.Fatalf("result = %+v, want cancelled only", result)
			}
		})
	}
}

func TestModeMenuCancelRowLeavesModeUnchosen(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{Ctx: ctx, Launch: true})
	m.Update(keyUp()) // wraps to the Cancel row
	_, cmd := m.Update(keyEnter())
	assertQuits(t, cmd)
	result := m.result()
	if !result.Cancelled || result.ModeChosen {
		t.Fatalf("result = %+v, want cancelled before a mode choice", result)
	}
}

func TestEmptySelectIsRecoverable(t *testing.T) {
	ctx := testContext(t)
	m := newWizard(WizardFlow{Ctx: ctx, Launch: true})
	m.Update(keyRune('1'))
	if m.step != stepSelect {
		t.Fatalf("step = %v, want an empty select screen", m.step)
	}
	for _, want := range []string{"(none detected)", "No supported harnesses were detected.", "Use --all"} {
		if !strings.Contains(flatText(m), want) {
			t.Errorf("empty select view missing %q:\n%s", want, viewText(m))
		}
	}

	for _, key := range []tea.KeyPressMsg{keyUp(), keyDown(), keySpace(), keyRune('a'), keyRune('n')} {
		if _, cmd := m.Update(key); cmd != nil {
			t.Fatalf("%v quit an empty select screen", key)
		}
	}
	if _, cmd := m.Update(keyEsc()); cmd != nil {
		t.Fatal("esc should return to the mode menu, not quit")
	}
	if m.step != stepMode {
		t.Fatalf("step = %v, want mode", m.step)
	}

	// Enter reports no changes instead of panicking or printing an empty table.
	fresh := newWizard(WizardFlow{Ctx: ctx, Launch: true})
	fresh.Update(keyRune('1'))
	_, cmd := fresh.Update(keyEnter())
	assertQuits(t, cmd)
	if result := fresh.result(); !result.NoChanges || result.Cancelled {
		t.Fatalf("result = %+v, want no changes", result)
	}
}

func TestEmptyMatrixVariantsAreRecoverable(t *testing.T) {
	ctx := testContext(t)
	harnesses := testHarnesses(ctx)
	cases := []struct {
		name        string
		matrix      *SkillsMatrix
		wantSkills  bool
		wantHarness bool
	}{
		{name: "nil", matrix: nil, wantSkills: true, wantHarness: true},
		{name: "no skills", matrix: &SkillsMatrix{Harnesses: harnesses}, wantSkills: true, wantHarness: false},
		{name: "no harnesses", matrix: &SkillsMatrix{Skills: []string{"alpha"}}, wantSkills: false, wantHarness: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: tc.matrix})
			m.Update(keyRune('2'))
			if m.step != stepMatrix {
				t.Fatalf("step = %v, want the matrix", m.step)
			}
			flat := flatText(m)
			if tc.wantSkills && !strings.Contains(flat, "(no shared skills found)") {
				t.Errorf("matrix view missing its placeholder:\n%s", viewText(m))
			}
			if tc.wantSkills && (!strings.Contains(flat, "Add <skill-name>/SKILL.md") || !strings.Contains(flat, "then rerun")) {
				t.Errorf("matrix view missing the skills hint:\n%s", viewText(m))
			}
			if tc.wantHarness && (!strings.Contains(flat, "No supported harnesses were detected.") || !strings.Contains(flat, "Use --all")) {
				t.Errorf("matrix view missing the harness hint:\n%s", viewText(m))
			}

			for _, key := range []tea.KeyPressMsg{keyUp(), keyDown(), keyLeft(), keyRight(), keySpace(), keyRune('a'), keyRune('n')} {
				if _, cmd := m.Update(key); cmd != nil {
					t.Fatalf("%v quit an empty matrix", key)
				}
			}
			if _, cmd := m.Update(keyEsc()); cmd != nil {
				t.Fatal("esc should return to the mode menu, not quit")
			}
			if m.step != stepMode {
				t.Fatalf("step = %v, want mode", m.step)
			}

			// Enter on the empty matrix reports no changes rather than
			// panicking or starting a zero-length plan.
			fresh := newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: tc.matrix})
			fresh.Update(keyRune('2'))
			_, cmd := fresh.Update(keyEnter())
			assertQuits(t, cmd)
			if result := fresh.result(); !result.NoChanges || result.Cancelled {
				t.Fatalf("result = %+v, want no changes", result)
			}
		})
	}
}

func TestLaunchHelpSaysBack(t *testing.T) {
	ctx := testContext(t)
	matrix, _ := testMatrix(t)

	cases := []struct {
		name  string
		model *wizard
		keys  []tea.KeyPressMsg
	}{
		{name: "select", model: newLaunchSelectWizard(t, ctx, nil), keys: []tea.KeyPressMsg{keyRune('1')}},
		{name: "matrix", model: newWizard(WizardFlow{Ctx: ctx, Launch: true, Skills: matrix, SkillsReview: skillsReview(nil)}), keys: []tea.KeyPressMsg{keyRune('2')}},
		{name: "setup menu", model: newWizard(WizardFlow{Ctx: ctx, Launch: true, Setup: true}), keys: []tea.KeyPressMsg{keyRune('1')}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range tc.keys {
				tc.model.Update(key)
			}
			view := viewText(tc.model)
			if !strings.Contains(view, "esc back") || !strings.Contains(view, "ctrl+c cancel") {
				t.Errorf("launch %s help = %q, want esc back and ctrl+c cancel", tc.name, view)
			}
		})
	}

	t.Run("standalone select", func(t *testing.T) {
		m := newSelectWizard(t, ctx)
		if view := viewText(m); !strings.Contains(view, "esc cancel") || strings.Contains(view, "esc back") {
			t.Errorf("standalone select help = %q, want esc cancel", view)
		}
	})
	t.Run("standalone matrix", func(t *testing.T) {
		m := newMatrixWizard(t, matrix, ctx)
		if view := viewText(m); !strings.Contains(view, "esc cancel") || strings.Contains(view, "esc back") {
			t.Errorf("standalone matrix help = %q, want esc cancel", view)
		}
	})
	t.Run("standalone setup menu", func(t *testing.T) {
		m := newWizard(WizardFlow{Ctx: ctx, Setup: true})
		if view := viewText(m); !strings.Contains(view, "esc cancel") || strings.Contains(view, "esc back") {
			t.Errorf("standalone setup help = %q, want esc cancel", view)
		}
	})
}

func TestConfirmModel(t *testing.T) {
	newModel := func() *ConfirmModel {
		return NewConfirmModel("Apply plan:\n  Create these symlinks")
	}

	t.Run("enter confirms", func(t *testing.T) {
		m := newModel()
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if !m.Confirmed || m.Cancelled {
			t.Fatalf("Confirmed = %v, Cancelled = %v; want Confirmed only", m.Confirmed, m.Cancelled)
		}
	})

	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{name: "esc", key: keyEsc()},
		{name: "ctrl+c", key: keyCtrlC()},
	} {
		t.Run(tc.name+" cancels", func(t *testing.T) {
			m := newModel()
			_, cmd := m.Update(tc.key)
			assertQuits(t, cmd)
			if m.Confirmed || !m.Cancelled {
				t.Fatalf("Confirmed = %v, Cancelled = %v; want Cancelled only", m.Confirmed, m.Cancelled)
			}
		})
	}

	t.Run("unescaped view shows the plan and the apply prompt", func(t *testing.T) {
		m := newModel()
		if !m.View().AltScreen {
			t.Error("confirm view is not declared as an alternate screen view")
		}
		view := viewText(m)
		if !strings.Contains(view, "Apply plan:") || !strings.Contains(view, "Apply this plan?") {
			t.Errorf("view is missing the plan or the apply prompt:\n%s", view)
		}
		if !strings.Contains(view, "enter apply") || !strings.Contains(view, "esc cancel") {
			t.Errorf("view is missing the key help:\n%s", view)
		}
	})

	t.Run("help toggles", func(t *testing.T) {
		m := newModel()
		m.Update(keyRune('?'))
		if !m.help.ShowAll {
			t.Error("? did not switch to the full help")
		}
	})
}

func TestSetupModelBlank(t *testing.T) {
	ctx := testContext(t)
	m := NewSetupModel(ctx)

	view := viewText(m)
	if !strings.Contains(view, "No shared instructions file found at") ||
		!strings.Contains(view, "Create a blank AGENTS.md") {
		t.Errorf("view is missing the missing-file line or the menu:\n%s", view)
	}

	_, cmd := m.Update(keyRune('2'))
	assertQuits(t, cmd)
	if m.Err != nil || !m.Created || m.From != "" || m.Cancelled {
		t.Fatalf("Created = %v, From = %q, Cancelled = %v, Err = %v", m.Created, m.From, m.Cancelled, m.Err)
	}
	contents, err := os.ReadFile(ctx.SourceFile)
	if err != nil || len(contents) != 0 {
		t.Fatalf("source contents = %q, %v; want empty", contents, err)
	}
}

func TestSetupModelCancel(t *testing.T) {
	ctx := testContext(t)

	for _, tc := range []struct {
		name string
		keys []tea.KeyPressMsg
	}{
		{name: "q", keys: []tea.KeyPressMsg{keyRune('q')}},
		{name: "esc", keys: []tea.KeyPressMsg{keyEsc()}},
		{name: "ctrl+c", keys: []tea.KeyPressMsg{keyCtrlC()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewSetupModel(ctx)
			var cmd tea.Cmd
			for _, key := range tc.keys {
				_, cmd = m.Update(key)
			}
			assertQuits(t, cmd)
			if m.Created || !m.Cancelled {
				t.Fatalf("Created = %v, Cancelled = %v; want Cancelled only", m.Created, m.Cancelled)
			}
			if _, err := os.Lstat(ctx.SourceFile); err == nil {
				t.Fatal("source file should not have been created")
			}
		})
	}
}

func TestSetupModelCopy(t *testing.T) {
	ctx := testContext(t)

	t.Run("copies a valid file", func(t *testing.T) {
		selected := filepath.Join(ctx.Home, "notes", "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(selected, []byte("# Shared\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		m := NewSetupModel(ctx)
		m.Update(keyRune('1'))
		if !strings.Contains(viewText(m), "Path to the existing instructions file") {
			t.Errorf("copy step not shown:\n%s", viewText(m))
		}
		// The typed path itself is textinput's business; set it directly.
		m.setup.input.SetValue(selected)
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if m.Err != nil || !m.Created || m.From != selected {
			t.Fatalf("Created = %v, From = %q, Err = %v", m.Created, m.From, m.Err)
		}
		contents, err := os.ReadFile(ctx.SourceFile)
		if err != nil || string(contents) != "# Shared\n" {
			t.Fatalf("source contents = %q, %v", contents, err)
		}
	})

	t.Run("invalid path keeps the prompt with a message", func(t *testing.T) {
		missing := filepath.Join(ctx.Home, "missing.md")
		m := NewSetupModel(ctx)
		m.Update(keyRune('1'))
		m.setup.input.SetValue(missing)
		_, cmd := m.Update(keyEnter())
		if cmd != nil {
			t.Fatal("invalid path should not quit the program")
		}
		if m.Err != nil || m.Created {
			t.Fatalf("Created = %v, Err = %v; want neither", m.Created, m.Err)
		}
		view := flatText(m)
		if !strings.Contains(view, "not found: "+missing) ||
			!strings.Contains(view, "Path to the existing instructions file") {
			t.Errorf("view does not show the error and the prompt:\n%s", viewText(m))
		}
	})

	t.Run("esc goes back to the menu", func(t *testing.T) {
		m := NewSetupModel(ctx)
		m.Update(keyRune('1'))
		_, cmd := m.Update(keyEsc())
		if cmd != nil || m.Cancelled {
			t.Fatalf("esc in the path step should return to the menu; Cancelled = %v", m.Cancelled)
		}
		if !strings.Contains(viewText(m), "Copy an existing instructions file") {
			t.Errorf("menu not shown after esc:\n%s", viewText(m))
		}
	})

	t.Run("enter on the menu's blank choice works", func(t *testing.T) {
		fresh := testContext(t)
		m := NewSetupModel(fresh)
		m.Update(keyDown())
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if !m.Created || m.From != "" {
			t.Fatalf("Created = %v, From = %q; want a blank creation", m.Created, m.From)
		}
		if _, err := os.Stat(fresh.SourceFile); err != nil {
			t.Fatalf("blank source file not created: %v", err)
		}
	})
}
