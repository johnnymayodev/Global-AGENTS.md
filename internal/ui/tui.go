package ui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// The Bubble Tea models behind the interactive paths. Each model is a plain
// value run by the caller with tea.NewProgram; the caller prints the final
// "Cancelled." / "Confirmation received" messages itself, after the program
// has restored the terminal and left the alternate screen.
//
// Terminal restoration is Bubble Tea's job: the pinned v2.1.0 renderer exits
// the alternate screen and re-enables the cursor in its shutdown path, which
// runs on normal exits, on the esc/ctrl+c handled by these models, on
// SIGINT/SIGTERM, and after a recovered panic.

// runProgram runs one model on the process terminal. The models handle
// ctrl+c and esc themselves, so a SIGINT-interrupted program is a
// cancellation, not a failure.
func runProgram(model tea.Model) error {
	if _, err := tea.NewProgram(model).Run(); err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return err
	}
	return nil
}

// PickerModel is the interactive multi-select harness table.
type PickerModel struct {
	harnesses []harness.Harness
	ctx       harness.Context
	selected  map[string]bool
	cursor    int

	// Done and Cancelled report how the program ended: enter sets Done,
	// esc/ctrl+c set Cancelled, and neither is set when the program was
	// killed externally.
	Done      bool
	Cancelled bool
}

// NewPickerModel starts a picker over the given harnesses with the initial
// selection (a set of harness IDs).
func NewPickerModel(harnesses []harness.Harness, ctx harness.Context, initial map[string]bool) *PickerModel {
	selected := make(map[string]bool, len(initial))
	for id, on := range initial {
		if on {
			selected[id] = true
		}
	}
	return &PickerModel{harnesses: harnesses, ctx: ctx, selected: selected}
}

// Selection returns a copy of the current selection.
func (m *PickerModel) Selection() map[string]bool {
	selection := make(map[string]bool, len(m.selected))
	for id, on := range m.selected {
		if on {
			selection[id] = true
		}
	}
	return selection
}

func (m *PickerModel) Init() tea.Cmd { return nil }

func (m *PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || len(m.harnesses) == 0 {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		m.cursor = (m.cursor + len(m.harnesses) - 1) % len(m.harnesses)
	case "down", "j":
		m.cursor = (m.cursor + 1) % len(m.harnesses)
	case "space":
		id := m.harnesses[m.cursor].ID
		if m.selected[id] {
			delete(m.selected, id)
		} else {
			m.selected[id] = true
		}
	case "a", "A":
		for _, h := range m.harnesses {
			m.selected[h.ID] = true
		}
	case "n", "N":
		m.selected = make(map[string]bool)
	case "enter":
		m.Done = true
		return m, tea.Quit
	case "esc", "ctrl+c":
		m.Cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

// pickerHelp is the shortcuts block under the table. j/k mirror the arrow
// keys, purely additively.
const pickerHelp = "↑/↓ or k/j move  Space toggle  A select all  N select none\n" +
	"Enter review  Esc cancel\n" +
	"Changes are applied only after reviewing and confirming the plan."

func (m *PickerModel) View() tea.View {
	table, err := RenderTable(m.harnesses, m.ctx, m.selected, m.cursor)
	if err != nil {
		// The plain path already rendered this table for the same paths;
		// keep the picker usable (esc still cancels) if a stat suddenly
		// fails.
		table = "error: " + err.Error()
	}
	v := tea.NewView(table + "\n\n" + pickerHelp)
	v.AltScreen = true
	return v
}

// ConfirmModel shows the plan text and a text input for the explicit "yes"
// approval.
type ConfirmModel struct {
	plan  string
	input textinput.Model

	// Confirmed is true only when the typed answer was "yes"; any other
	// answer, esc, or ctrl+c counts as a decline.
	Confirmed bool
	Cancelled bool
}

// NewConfirmModel builds the confirmation prompt for a plan.
func NewConfirmModel(planText, prompt string) *ConfirmModel {
	input := textinput.New()
	input.Prompt = prompt
	return &ConfirmModel{plan: planText, input: input}
}

func (m *ConfirmModel) Init() tea.Cmd {
	return m.input.Focus()
}

func (m *ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.Confirmed = strings.EqualFold(strings.TrimSpace(m.input.Value()), "yes")
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.Cancelled = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *ConfirmModel) View() tea.View {
	v := tea.NewView(m.plan + "\n\n" + m.input.View())
	v.AltScreen = true
	return v
}

type setupStep int

const (
	setupMenu setupStep = iota
	setupCopyPath
)

// SetupModel is the missing-source-file flow: the copy/blank/cancel menu and
// the path prompt for the copy case, as one program.
type SetupModel struct {
	ctx   harness.Context
	step  setupStep
	input textinput.Model

	message string // validation feedback shown above the menu

	// Created reports that the canonical instructions file was created.
	// From is the copied file; when Created and From is empty the file was
	// created blank.
	Created   bool
	From      string
	Cancelled bool
	Err       error
}

// NewSetupModel starts the setup flow for a missing canonical instructions
// file.
func NewSetupModel(ctx harness.Context) *SetupModel {
	return &SetupModel{ctx: ctx, input: textinput.New()}
}

func (m *SetupModel) Init() tea.Cmd { return nil }

func (m *SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch m.step {
	case setupMenu:
		switch key.String() {
		case "1":
			m.step = setupCopyPath
			m.message = ""
			m.input.SetValue("")
			return m, m.input.Focus()
		case "2":
			if err := CreateBlankSource(m.ctx.SourceFile); err != nil {
				m.Err = err
			} else {
				m.Created = true
			}
			return m, tea.Quit
		case "q", "esc", "ctrl+c":
			m.Cancelled = true
			return m, tea.Quit
		default:
			m.message = `Enter "1" to copy a file, "2" to create a blank file, or "q" to cancel.`
			return m, nil
		}
	case setupCopyPath:
		switch key.String() {
		case "enter":
			return m.copySelected()
		case "esc":
			m.step = setupMenu
			m.message = ""
			m.input.Blur()
			return m, nil
		case "ctrl+c":
			m.Cancelled = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// copySelected resolves and copies the path the user typed, returning to the
// menu with a message when it is not a readable regular file.
func (m *SetupModel) copySelected() (tea.Model, tea.Cmd) {
	selected := SelectedFilePath(m.input.Value(), m.ctx.Home, m.ctx.Cwd())
	if err := CheckSelectedFile(selected); err != nil {
		m.step = setupMenu
		m.message = err.Error()
		m.input.Blur()
		return m, nil
	}
	if err := CopySourceFile(m.ctx.SourceFile, selected); err != nil {
		m.Err = err
		return m, tea.Quit
	}
	m.From = selected
	m.Created = true
	return m, tea.Quit
}

func (m *SetupModel) View() tea.View {
	lines := []string{
		fmt.Sprintf("No shared instructions file found at %s.", m.ctx.SourceFile),
		"",
	}
	if m.message != "" {
		lines = append(lines, m.message, "")
	}
	if m.step == setupCopyPath {
		lines = append(lines, "Path to the existing instructions file: "+m.input.View())
	} else {
		lines = append(lines, "Choose: [1] copy an existing instructions file  [2] create a blank AGENTS.md  [q] cancel: ")
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
