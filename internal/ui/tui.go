package ui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// Key bindings shared by the interactive screens. Match bindings stay
// separate from the help bindings so one binding can never match two
// directions; the help model only renders the help side.
var (
	keyUpward   = key.NewBinding(key.WithKeys("up", "k"))
	keyDownward = key.NewBinding(key.WithKeys("down", "j"))
	keyMove     = key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "move"))
	keyToggle   = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle"))
	keyAll      = key.NewBinding(key.WithKeys("a", "A"), key.WithHelp("a", "select all"))
	keyNone     = key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "select none"))
	keyReview   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review"))
	keyChoose   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose"))
	keyContinue = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "continue"))
	keyApply    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply"))
	keyBack     = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyCancel   = key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel"))
	keyQuit     = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel"))
	keyHelp     = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
)

// selectKeys is the select step's help layout.
type selectKeys struct{}

func (selectKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyMove, keyToggle, keyReview, keyCancel, keyHelp}
}

func (selectKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyMove, keyToggle, keyAll, keyNone},
		{keyReview, keyCancel, keyHelp},
	}
}

// reviewKeys is the review step's help layout: one keypress applies.
type reviewKeys struct{}

func (reviewKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyApply, keyBack, keyQuit, keyHelp}
}

func (reviewKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyApply},
		{keyBack, keyQuit, keyHelp},
	}
}

// setupMenuKeys is the setup menu's help layout.
type setupMenuKeys struct{}

func (setupMenuKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyMove, keyChoose, keyCancel}
}

func (setupMenuKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyMove},
		{keyChoose, keyCancel},
	}
}

// setupPathKeys is the copy-path step's help layout.
type setupPathKeys struct{}

func (setupPathKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyContinue, keyBack, keyQuit}
}

func (setupPathKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyContinue},
		{keyBack, keyQuit},
	}
}

// confirmKeys is the standalone review program's help layout: with no select
// step to return to, esc cancels the whole run.
type confirmKeys struct{}

func (confirmKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyApply, keyCancel, keyHelp}
}

func (confirmKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyApply},
		{keyCancel, keyHelp},
	}
}

// Review is a planned change the wizard shows on its review step. The caller
// builds Text from the same plan its Apply closure executes, so exactly what
// was shown is applied after the program exits.
type Review struct {
	Text    string
	Summary string
	Apply   func() error
}

// WizardFlow configures one run of the interactive wizard.
type WizardFlow struct {
	Harnesses []harness.Harness
	Ctx       harness.Context
	Initial   map[string]bool

	// Setup starts the wizard on the first-run setup step, for a missing
	// shared instructions file.
	Setup bool

	// Review turns a selection into a review. ok is false when the
	// selection leads to no changes at all.
	Review func(selection map[string]bool) (review Review, ok bool, err error)
}

// WizardResult reports how the wizard ended. The caller prints the outcome
// after the program has left the alternate screen.
type WizardResult struct {
	Notices   []string // e.g. "Created ..." lines from the setup step
	Cancelled bool
	NoChanges bool
	Applied   bool
	Review    Review
}

// RunWizard runs setup (when needed), selection, and review as one
// full-screen program.
func RunWizard(flow WizardFlow) (WizardResult, error) {
	m := newWizard(flow)
	if err := runProgram(m); err != nil {
		return WizardResult{}, err
	}
	if m.err != nil {
		return WizardResult{}, m.err
	}
	result := m.result()
	if !result.Applied && !result.Cancelled && !result.NoChanges {
		// The program ended without finishing a step (a SIGTERM the
		// terminal restore handled): the caller should report it as a
		// cancellation, like any other interrupted picker.
		result.Cancelled = true
	}
	return result, nil
}

// wizardStep identifies the wizard's current screen.
type wizardStep int

const (
	stepSetup wizardStep = iota
	stepSelect
	stepReview
)

// wizard is the single program behind the picker flow: the setup step (when
// the shared instructions file is missing), the harness table, and the plan
// review.
type wizard struct {
	flow  WizardFlow
	step  wizardStep
	setup setupState
	help  help.Model
	dark  bool

	width int

	cursor   int
	selected map[string]bool

	review Review

	notices   []string
	applied   bool
	cancelled bool
	noChanges bool
	err       error
}

func newWizard(flow WizardFlow) *wizard {
	selected := make(map[string]bool, len(flow.Initial))
	for id, on := range flow.Initial {
		if on {
			selected[id] = true
		}
	}
	m := &wizard{
		flow:     flow,
		step:     stepSelect,
		setup:    newSetupState(flow.Ctx),
		help:     help.New(),
		dark:     true,
		width:    80,
		selected: selected,
	}
	if flow.Setup {
		m.step = stepSetup
	}
	applyHelpStyles(&m.help, m.dark)
	return m
}

func (m *wizard) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Selection returns a copy of the current selection.
func (m *wizard) Selection() map[string]bool {
	selection := make(map[string]bool, len(m.selected))
	for id, on := range m.selected {
		if on {
			selection[id] = true
		}
	}
	return selection
}

func (m *wizard) result() WizardResult {
	return WizardResult{
		Notices:   m.notices,
		Cancelled: m.cancelled,
		NoChanges: m.noChanges,
		Applied:   m.applied,
		Review:    m.review,
	}
}

func (m *wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.setup.setDark(m.dark)
		applyHelpStyles(&m.help, m.dark)
		return m, nil
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.step == stepSetup && m.setup.step == setupCopyPath {
		var cmd tea.Cmd
		m.setup.input, cmd = m.setup.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *wizard) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case stepSetup:
		return m.updateSetup(msg)
	case stepSelect:
		return m.updateSelect(msg)
	default:
		return m.updateReview(msg)
	}
}

// updateSetup drives the shared setup flow; a created file moves the wizard
// on to the selection step, with the outcome line queued for after the
// program exits.
func (m *wizard) updateSetup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	event, cmd, err := m.setup.handleKey(msg)
	switch event {
	case setupCreatedBlank:
		m.notices = append(m.notices, fmt.Sprintf("Created blank instructions file: %s", m.setup.ctx.SourceFile))
		m.step = stepSelect
	case setupCreatedFrom:
		m.notices = append(m.notices, fmt.Sprintf("Created %s from %s", m.setup.ctx.SourceFile, m.setup.copied))
		m.step = stepSelect
	case setupCancelled:
		m.cancelled = true
		return m, tea.Quit
	case setupFailed:
		m.err = err
		return m, tea.Quit
	}
	return m, cmd
}

func (m *wizard) updateSelect(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if len(m.flow.Harnesses) == 0 {
		m.cancelled = true
		return m, tea.Quit
	}
	switch {
	case key.Matches(msg, keyUpward):
		m.cursor = (m.cursor + len(m.flow.Harnesses) - 1) % len(m.flow.Harnesses)
	case key.Matches(msg, keyDownward):
		m.cursor = (m.cursor + 1) % len(m.flow.Harnesses)
	case key.Matches(msg, keyToggle):
		id := m.flow.Harnesses[m.cursor].ID
		if m.selected[id] {
			delete(m.selected, id)
		} else {
			m.selected[id] = true
		}
	case key.Matches(msg, keyAll):
		for _, h := range m.flow.Harnesses {
			m.selected[h.ID] = true
		}
	case key.Matches(msg, keyNone):
		m.selected = make(map[string]bool)
	case key.Matches(msg, keyReview):
		return m.reviewSelection()
	case key.Matches(msg, keyCancel):
		m.cancelled = true
		return m, tea.Quit
	case key.Matches(msg, keyHelp):
		m.help.ShowAll = !m.help.ShowAll
	}
	return m, nil
}

// reviewSelection asks the caller to build the review for the current
// selection; no changes at all ends the wizard instead.
func (m *wizard) reviewSelection() (tea.Model, tea.Cmd) {
	review, ok, err := m.flow.Review(m.Selection())
	if err != nil {
		m.err = err
		return m, tea.Quit
	}
	if !ok {
		m.noChanges = true
		return m, tea.Quit
	}
	m.review = review
	m.step = stepReview
	return m, nil
}

func (m *wizard) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyApply):
		m.applied = true
		return m, tea.Quit
	case key.Matches(msg, keyBack):
		m.step = stepSelect
	case key.Matches(msg, keyQuit):
		m.cancelled = true
		return m, tea.Quit
	case key.Matches(msg, keyHelp):
		m.help.ShowAll = !m.help.ShowAll
	}
	return m, nil
}

// statusLine reports how many harnesses are selected.
func (m *wizard) statusLine(t theme) string {
	selected := 0
	for _, on := range m.selected {
		if on {
			selected++
		}
	}
	return t.faint.Render("Selected") + " " + t.bold.Render(fmt.Sprintf("%d of %d", selected, len(m.flow.Harnesses)))
}

func (m *wizard) View() tea.View {
	pad, inner := cardLayout(m.width)
	t := newTheme(m.dark)
	var (
		subtitle string
		sections []string
		helpText string
	)
	switch m.step {
	case stepSetup:
		subtitle = "First run."
		sections = m.setup.sections(t, inner)
		if m.setup.step == setupCopyPath {
			helpText = t.helpLine(m.help, setupPathKeys{}, inner)
		} else {
			helpText = t.helpLine(m.help, setupMenuKeys{}, inner)
		}
	case stepSelect:
		subtitle = "Link your shared instructions into each harness."
		table, err := renderHarnessTable(t, m.flow.Harnesses, m.flow.Ctx, m.selected, m.cursor, inner)
		if err != nil {
			// Keep the picker usable (esc still cancels) if a stat fails.
			table = t.danger.Render("error: " + err.Error())
		}
		sections = []string{table, "", m.statusLine(t)}
		helpText = t.helpLine(m.help, selectKeys{}, inner)
	default:
		subtitle = "Review and confirm."
		sections = []string{
			t.plan.Width(inner).Render(m.review.Text),
			"",
			t.accent.Render("❯") + " " + t.bold.Render("Apply this plan?"),
		}
		helpText = t.helpLine(m.help, reviewKeys{}, inner)
	}
	v := tea.NewView(t.card(pad, inner, subtitle, sections, helpText))
	v.AltScreen = true
	return v
}

// setupStep identifies the setup flow's current screen.
type setupStep int

const (
	setupMenu setupStep = iota
	setupCopyPath
)

// setupMenuItems are the first-run choices; the indexes matter, the 1/2/q
// accelerators map onto them.
var setupMenuItems = []string{
	"Copy an existing instructions file",
	"Create a blank AGENTS.md",
	"Cancel",
}

// setupEvent reports what one key did to the setup flow.
type setupEvent int

const (
	setupPending setupEvent = iota
	setupCreatedBlank
	setupCreatedFrom
	setupCancelled
	setupFailed
)

// setupState is the shared first-run setup flow: the copy / blank / cancel
// menu and the path prompt with live validation. The wizard embeds it as its
// first step; SetupModel drives it as a standalone program.
type setupState struct {
	ctx    harness.Context
	step   setupStep
	menu   int
	input  textinput.Model
	dark   bool
	copied string // the file a successful copy came from
}

func newSetupState(ctx harness.Context) setupState {
	input := textinput.New()
	input.Prompt = "❯ "
	input.Validate = func(value string) error {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		return CheckSelectedFile(SelectedFilePath(value, ctx.Home, ctx.Cwd()))
	}
	applyInputStyles(&input, true)
	return setupState{ctx: ctx, input: input, dark: true}
}

func (s *setupState) setDark(dark bool) {
	s.dark = dark
	applyInputStyles(&s.input, dark)
}

// handleKey advances the setup flow. The returned command belongs to the
// focused input (its cursor blink); callers run it.
func (s *setupState) handleKey(msg tea.KeyPressMsg) (setupEvent, tea.Cmd, error) {
	if s.step == setupCopyPath {
		switch {
		case key.Matches(msg, keyContinue):
			return s.copySelected()
		case key.Matches(msg, keyBack):
			s.step = setupMenu
			s.input.Blur()
			return setupPending, nil, nil
		case key.Matches(msg, keyQuit):
			return setupCancelled, nil, nil
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return setupPending, cmd, nil
	}

	switch {
	case key.Matches(msg, keyUpward):
		s.menu = (s.menu + len(setupMenuItems) - 1) % len(setupMenuItems)
		return setupPending, nil, nil
	case key.Matches(msg, keyDownward):
		s.menu = (s.menu + 1) % len(setupMenuItems)
		return setupPending, nil, nil
	case key.Matches(msg, keyChoose):
		return s.choose()
	}
	switch msg.String() {
	case "1":
		s.menu = 0
		return s.choose()
	case "2":
		s.menu = 1
		return s.choose()
	case "q", "esc", "ctrl+c":
		return setupCancelled, nil, nil
	}
	return setupPending, nil, nil
}

func (s *setupState) choose() (setupEvent, tea.Cmd, error) {
	switch s.menu {
	case 0:
		s.step = setupCopyPath
		s.input.SetValue("")
		return setupPending, s.input.Focus(), nil
	case 1:
		if err := CreateBlankSource(s.ctx.SourceFile); err != nil {
			return setupFailed, nil, err
		}
		return setupCreatedBlank, nil, nil
	default:
		return setupCancelled, nil, nil
	}
}

// copySelected copies the typed path, re-checking it at submit time so a file
// that changed while the user typed cannot slip through.
func (s *setupState) copySelected() (setupEvent, tea.Cmd, error) {
	value := strings.TrimSpace(s.input.Value())
	if value == "" {
		return setupPending, nil, nil
	}
	selected := SelectedFilePath(value, s.ctx.Home, s.ctx.Cwd())
	if err := CheckSelectedFile(selected); err != nil {
		s.input.Err = err
		return setupPending, nil, nil
	}
	if err := CopySourceFile(s.ctx.SourceFile, selected); err != nil {
		return setupFailed, nil, err
	}
	s.copied = selected
	return setupCreatedFrom, nil, nil
}

// sections renders the setup screens' content.
func (s *setupState) sections(t theme, inner int) []string {
	if s.step == setupCopyPath {
		lines := []string{
			t.bold.Render("Path to the existing instructions file"),
			s.input.View(),
		}
		if validity := s.validityLine(t); validity != "" {
			lines = append(lines, validity)
		}
		return lines
	}

	lines := []string{
		lipgloss.NewStyle().Width(inner).Render(fmt.Sprintf("No shared instructions file found at %s.", s.ctx.SourceFile)),
		"",
	}
	for i, item := range setupMenuItems {
		if i == s.menu {
			lines = append(lines, t.bar.Render(padRight("❯ "+item, inner)))
			continue
		}
		lines = append(lines, "  "+item)
	}
	return lines
}

// validityLine is the live feedback under the path field: a green check for a
// readable file, the validation error otherwise.
func (s *setupState) validityLine(t theme) string {
	if strings.TrimSpace(s.input.Value()) == "" {
		return ""
	}
	if err := s.input.Err; err != nil {
		return t.danger.Render(err.Error())
	}
	return t.green.Render("✓ file exists")
}

// ConfirmModel is the standalone review program used by link and unlink: it
// shows the plan and applies it on a single keypress.
type ConfirmModel struct {
	plan  string
	help  help.Model
	dark  bool
	width int

	// Confirmed reports that the user pressed enter; esc and ctrl+c cancel.
	Confirmed bool
	Cancelled bool
}

// NewConfirmModel builds the confirmation prompt for a plan.
func NewConfirmModel(planText string) *ConfirmModel {
	m := &ConfirmModel{plan: planText, help: help.New(), dark: true, width: 80}
	applyHelpStyles(&m.help, m.dark)
	return m
}

func (m *ConfirmModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m *ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		applyHelpStyles(&m.help, m.dark)
		return m, nil
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyApply):
			m.Confirmed = true
			return m, tea.Quit
		case key.Matches(msg, keyBack), key.Matches(msg, keyQuit):
			m.Cancelled = true
			return m, tea.Quit
		case key.Matches(msg, keyHelp):
			m.help.ShowAll = !m.help.ShowAll
		}
		return m, nil
	}
	return m, nil
}

func (m *ConfirmModel) View() tea.View {
	pad, inner := cardLayout(m.width)
	t := newTheme(m.dark)
	sections := []string{
		t.plan.Width(inner).Render(m.plan),
		"",
		t.accent.Render("❯") + " " + t.bold.Render("Apply this plan?"),
	}
	v := tea.NewView(t.card(pad, inner, "Review and confirm.", sections, t.helpLine(m.help, confirmKeys{}, inner)))
	v.AltScreen = true
	return v
}

// SetupModel is the standalone setup program: the first-run menu and path
// prompt for the runs that will not go on to a picker (link, or a picker with
// nothing detected). The wizard embeds the same flow.
type SetupModel struct {
	setup setupState
	help  help.Model
	width int

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
	m := &SetupModel{setup: newSetupState(ctx), help: help.New(), width: 80}
	applyHelpStyles(&m.help, true)
	return m
}

func (m *SetupModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m *SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.setup.setDark(msg.IsDark())
		applyHelpStyles(&m.help, msg.IsDark())
		return m, nil
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyPressMsg:
		event, cmd, err := m.setup.handleKey(msg)
		switch event {
		case setupCreatedBlank:
			m.Created = true
			return m, tea.Quit
		case setupCreatedFrom:
			m.Created = true
			m.From = m.setup.copied
			return m, tea.Quit
		case setupCancelled:
			m.Cancelled = true
			return m, tea.Quit
		case setupFailed:
			m.Err = err
			return m, tea.Quit
		}
		return m, cmd
	}
	if m.setup.step == setupCopyPath {
		var cmd tea.Cmd
		m.setup.input, cmd = m.setup.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *SetupModel) View() tea.View {
	pad, inner := cardLayout(m.width)
	t := newTheme(m.setup.dark)
	var keys help.KeyMap = setupMenuKeys{}
	if m.setup.step == setupCopyPath {
		keys = setupPathKeys{}
	}
	v := tea.NewView(t.card(pad, inner, "First run.", m.setup.sections(t, inner), t.helpLine(m.help, keys, inner)))
	v.AltScreen = true
	return v
}
