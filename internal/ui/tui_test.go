package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// The model tests drive Update with key messages and assert on the model
// state and View() content, without a terminal.

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

// pickerFocus extracts the name of the harness on the focused row.
func pickerFocus(t *testing.T, m *PickerModel) string {
	t.Helper()
	for _, line := range strings.Split(m.View().Content, "\n") {
		if !strings.HasPrefix(line, "│ > │") {
			continue
		}
		for _, h := range m.harnesses {
			if strings.Contains(line, "[") && strings.Contains(line, " "+h.Name+" ") {
				return h.Name
			}
		}
	}
	t.Fatalf("no focused row in view:\n%s", m.View().Content)
	return ""
}

func TestPickerCursorMovement(t *testing.T) {
	ctx := testContext(t)
	harnesses := testHarnesses(ctx)
	m := NewPickerModel(harnesses, ctx, nil)

	if got := pickerFocus(t, m); got != "Alpha" {
		t.Fatalf("initial focus = %q, want Alpha", got)
	}

	m.Update(keyDown())
	if got := pickerFocus(t, m); got != "Beta" {
		t.Fatalf("after down = %q, want Beta", got)
	}
	m.Update(keyDown())
	m.Update(keyDown())
	if got := pickerFocus(t, m); got != "Alpha" {
		t.Fatalf("after wrapping down = %q, want Alpha", got)
	}
	m.Update(keyUp())
	if got := pickerFocus(t, m); got != "Gamma" {
		t.Fatalf("after wrapping up = %q, want Gamma", got)
	}
}

func TestPickerVimAliases(t *testing.T) {
	ctx := testContext(t)
	harnesses := testHarnesses(ctx)
	m := NewPickerModel(harnesses, ctx, nil)

	m.Update(keyRune('j'))
	if got := pickerFocus(t, m); got != "Beta" {
		t.Fatalf("after j = %q, want Beta", got)
	}
	m.Update(keyRune('k'))
	if got := pickerFocus(t, m); got != "Alpha" {
		t.Fatalf("after k = %q, want Alpha", got)
	}
	m.Update(keyRune('k'))
	if got := pickerFocus(t, m); got != "Gamma" {
		t.Fatalf("after wrapping k = %q, want Gamma", got)
	}
}

func TestPickerToggle(t *testing.T) {
	ctx := testContext(t)
	m := NewPickerModel(testHarnesses(ctx), ctx, map[string]bool{"beta": true})

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
	if !strings.Contains(m.View().Content, "[x] Alpha") || !strings.Contains(m.View().Content, "[ ] Beta") {
		t.Errorf("view does not reflect the selection:\n%s", m.View().Content)
	}
}

func TestPickerSelectAllAndNone(t *testing.T) {
	ctx := testContext(t)
	m := NewPickerModel(testHarnesses(ctx), ctx, nil)

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

func TestPickerSelectionIsACopy(t *testing.T) {
	ctx := testContext(t)
	m := NewPickerModel(testHarnesses(ctx), ctx, nil)
	m.Update(keySpace())

	selection := m.Selection()
	selection["alpha"] = false
	if !m.Selection()["alpha"] {
		t.Fatal("mutating the returned selection changed the model")
	}
}

func TestPickerEnterAndCancel(t *testing.T) {
	ctx := testContext(t)
	harnesses := testHarnesses(ctx)

	t.Run("enter reviews the selection", func(t *testing.T) {
		m := NewPickerModel(harnesses, ctx, nil)
		m.Update(keySpace())
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if !m.Done || m.Cancelled {
			t.Fatalf("Done = %v, Cancelled = %v; want Done only", m.Done, m.Cancelled)
		}
		if !m.Selection()["alpha"] {
			t.Fatal("selection was not kept")
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
			m := NewPickerModel(harnesses, ctx, nil)
			_, cmd := m.Update(tc.key)
			assertQuits(t, cmd)
			if m.Done || !m.Cancelled {
				t.Fatalf("Done = %v, Cancelled = %v; want Cancelled only", m.Done, m.Cancelled)
			}
		})
	}
}

func TestPickerView(t *testing.T) {
	ctx := testContext(t)
	m := NewPickerModel(testHarnesses(ctx), ctx, nil)
	view := m.View()

	if !view.AltScreen {
		t.Error("picker view is not declared as an alternate screen view")
	}
	for _, want := range []string{"Harness", "[ ] Alpha", "Space toggle", "Enter review", "Esc cancel"} {
		if !strings.Contains(view.Content, want) {
			t.Errorf("view is missing %q:\n%s", want, view.Content)
		}
	}
}

func TestConfirmModel(t *testing.T) {
	prompt := `Type "yes" to apply the plan above (create 1 symlink(s)): `
	newModel := func() *ConfirmModel {
		m := NewConfirmModel("Apply plan:\n  Create these symlinks", prompt)
		m.Init() // focuses the input, as the program does
		return m
	}

	t.Run("yes confirms", func(t *testing.T) {
		m := newModel()
		for _, r := range "yes" {
			m.Update(keyRune(r))
		}
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if !m.Confirmed || m.Cancelled {
			t.Fatalf("Confirmed = %v, Cancelled = %v; want Confirmed only", m.Confirmed, m.Cancelled)
		}
	})

	t.Run("trimmed uppercase yes confirms", func(t *testing.T) {
		m := newModel()
		m.Update(keySpace())
		for _, r := range "YES" {
			m.Update(keyRune(r))
		}
		m.Update(keySpace())
		m.Update(keyEnter())
		if !m.Confirmed {
			t.Fatal("wanted a confirmation for \" YES \"")
		}
	})

	t.Run("other answers decline", func(t *testing.T) {
		m := newModel()
		for _, r := range "no" {
			m.Update(keyRune(r))
		}
		_, cmd := m.Update(keyEnter())
		assertQuits(t, cmd)
		if m.Confirmed || m.Cancelled {
			t.Fatalf("Confirmed = %v, Cancelled = %v; want a plain decline", m.Confirmed, m.Cancelled)
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

	t.Run("view shows the plan and the prompt", func(t *testing.T) {
		view := newModel().View().Content
		if !strings.Contains(view, "Apply plan:") || !strings.Contains(view, prompt) {
			t.Errorf("view is missing the plan or prompt:\n%s", view)
		}
	})
}

func TestSetupModelBlank(t *testing.T) {
	ctx := testContext(t)
	m := NewSetupModel(ctx)

	if !strings.Contains(m.View().Content, "No shared instructions file found at "+ctx.SourceFile) {
		t.Errorf("view is missing the missing-file line:\n%s", m.View().Content)
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
		if !strings.Contains(m.View().Content, "Path to the existing instructions file: ") {
			t.Errorf("copy step not shown:\n%s", m.View().Content)
		}
		// The typed path itself is textinput's business; set it directly.
		m.input.SetValue(selected)
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

	t.Run("invalid path returns to the menu with a message", func(t *testing.T) {
		missing := filepath.Join(ctx.Home, "missing.md")
		m := NewSetupModel(ctx)
		m.Update(keyRune('1'))
		m.input.SetValue(missing)
		_, cmd := m.Update(keyEnter())
		if cmd != nil {
			t.Fatal("invalid path should not quit the program")
		}
		if m.Err != nil || m.Created {
			t.Fatalf("Created = %v, Err = %v; want neither", m.Created, m.Err)
		}
		view := m.View().Content
		if !strings.Contains(view, "not found: "+missing) || !strings.Contains(view, "Choose: [1] copy") {
			t.Errorf("view does not show the error and the menu:\n%s", view)
		}
	})

	t.Run("esc goes back to the menu", func(t *testing.T) {
		m := NewSetupModel(ctx)
		m.Update(keyRune('1'))
		_, cmd := m.Update(keyEsc())
		if cmd != nil || m.Cancelled {
			t.Fatalf("esc in the path step should return to the menu; Cancelled = %v", m.Cancelled)
		}
		if !strings.Contains(m.View().Content, "Choose: [1] copy") {
			t.Errorf("menu not shown after esc:\n%s", m.View().Content)
		}
	})
}
