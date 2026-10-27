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
	keyUpward    = key.NewBinding(key.WithKeys("up", "k"))
	keyDownward  = key.NewBinding(key.WithKeys("down", "j"))
	keyLeftward  = key.NewBinding(key.WithKeys("left", "h"))
	keyRightward = key.NewBinding(key.WithKeys("right", "l"))
	keyMove      = key.NewBinding(key.WithKeys("up", "down", "k", "j"), key.WithHelp("↑/↓", "move"))
	keyMoveCells = key.NewBinding(key.WithKeys("left", "right", "h", "l"), key.WithHelp("←/→", "harness"))
	keyToggle    = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle"))
	keyAll       = key.NewBinding(key.WithKeys("a", "A"), key.WithHelp("a", "select all"))
	keyNone      = key.NewBinding(key.WithKeys("n", "N"), key.WithHelp("n", "select none"))
	keyReview    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "review"))
	keyChoose    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose"))
	keyContinue  = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "continue"))
	keyApply     = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply"))
	keyBack      = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyCancel    = key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "cancel"))
	keyQuit      = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel"))
	keyHelp      = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	keyPageUp    = key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup/pgdown", "scroll"))
	keyPageDown  = key.NewBinding(key.WithKeys("pgdown"))
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
	return []key.Binding{keyApply, keyPageUp, keyBack, keyQuit, keyHelp}
}

func (reviewKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyApply},
		{keyPageUp, keyBack, keyQuit, keyHelp},
	}
}

// matrixKeys is the skills matrix's help layout.
type matrixKeys struct{}

func (matrixKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyMove, keyToggle, keyReview, keyCancel, keyHelp}
}

func (matrixKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyMove, keyMoveCells},
		{keyToggle, keyAll, keyNone},
		{keyReview, keyCancel, keyHelp},
	}
}

// modeKeys is the launch mode menu's help layout.
type modeKeys struct{}

func (modeKeys) ShortHelp() []key.Binding {
	return []key.Binding{keyMove, keyChoose, keyCancel}
}

func (modeKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyMove},
		{keyChoose, keyCancel},
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

// Mode selects which kind of global configuration the wizard manages.
type Mode int

const (
	// ModeInstructions manages the shared AGENTS.md file.
	ModeInstructions Mode = iota
	// ModeSkills manages the shared skills directory.
	ModeSkills
)

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

	// Launch starts on a mode menu instead of a specific flow.
	Launch bool

	// InstructionsError and SkillsError defer a preparation failure to the
	// mode choice, so a broken source for one mode never blocks the other.
	InstructionsError error
	SkillsError       error

	// Mode selects instructions or skills when Launch is false.
	Mode Mode

	// Skills is the prepared matrix for skills mode. It may be nil when
	// only the instructions side is reachable.
	Skills *SkillsMatrix

	// SkillsReview turns a matrix selection into a review.
	SkillsReview func(selection map[string]bool) (review Review, ok bool, err error)
}

// WizardResult reports how the wizard ended. The caller prints the outcome
// after the program has left the alternate screen.
type WizardResult struct {
	Notices    []string // e.g. "Created ..." lines from the setup step
	Mode       Mode
	ModeChosen bool
	Cancelled  bool
	NoChanges  bool
	Applied    bool
	Review     Review
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
	stepMode wizardStep = iota
	stepSetup
	stepSelect
	stepReview
	stepMatrix
)

// wizard is the single program behind the picker flow: the launch mode menu,
// the setup step (when the shared instructions file is missing), the harness
// table, the skills matrix, and the plan review.
type wizard struct {
	flow  WizardFlow
	step  wizardStep
	setup setupState
	help  help.Model
	dark  bool

	width  int
	height int

	cursor   int
	selected map[string]bool

	modeMenu int

	matrixRow      int
	matrixCol      int
	matrixSelected map[string]bool

	review       Review
	reviewOffset int

	notices    []string
	mode       Mode
	modeChosen bool
	applied    bool
	cancelled  bool
	noChanges  bool
	err        error
}

func newWizard(flow WizardFlow) *wizard {
	selected := make(map[string]bool, len(flow.Initial))
	for id, on := range flow.Initial {
		if on {
			selected[id] = true
		}
	}
	matrixSelected := make(map[string]bool)
	if flow.Skills != nil {
		for key, on := range flow.Skills.Initial {
			if on {
				matrixSelected[key] = true
			}
		}
	}
	m := &wizard{
		flow:           flow,
		setup:          newSetupState(flow.Ctx),
		help:           help.New(),
		dark:           true,
		width:          80,
		height:         24,
		selected:       selected,
		matrixSelected: matrixSelected,
		mode:           flow.Mode,
		modeChosen:     !flow.Launch,
	}
	switch {
	case flow.Launch:
		m.step = stepMode
	case flow.Mode == ModeSkills:
		m.step = stepMatrix
	case flow.Setup:
		m.step = stepSetup
	default:
		m.step = stepSelect
	}
	applyHelpStyles(&m.help, m.dark)
	return m
}

func (m *wizard) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Selection returns a copy of the current instructions selection.
func (m *wizard) Selection() map[string]bool {
	selection := make(map[string]bool, len(m.selected))
	for id, on := range m.selected {
		if on {
			selection[id] = true
		}
	}
	return selection
}

// MatrixSelection returns a copy of the current skills selection.
func (m *wizard) MatrixSelection() map[string]bool {
	selection := make(map[string]bool, len(m.matrixSelected))
	for key, on := range m.matrixSelected {
		if on {
			selection[key] = true
		}
	}
	return selection
}

func (m *wizard) result() WizardResult {
	return WizardResult{
		Notices:    m.notices,
		Mode:       m.mode,
		ModeChosen: m.modeChosen,
		Cancelled:  m.cancelled,
		NoChanges:  m.noChanges,
		Applied:    m.applied,
		Review:     m.review,
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
		if msg.Height > 0 {
			m.height = msg.Height
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
	case stepMode:
		return m.updateMode(msg)
	case stepSetup:
		return m.updateSetup(msg)
	case stepSelect:
		return m.updateSelect(msg)
	case stepMatrix:
		return m.updateMatrix(msg)
	default:
		return m.updateReview(msg)
	}
}

// modeMenuItems are the launch choices; the indexes matter, the 1/2/q
// accelerators map onto them.
var modeMenuItems = []string{
	"Manage global instructions (AGENTS.md)",
	"Manage global skills (SKILL.md)",
	"Cancel",
}

func (m *wizard) updateMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyUpward):
		m.modeMenu = (m.modeMenu + len(modeMenuItems) - 1) % len(modeMenuItems)
		return m, nil
	case key.Matches(msg, keyDownward):
		m.modeMenu = (m.modeMenu + 1) % len(modeMenuItems)
		return m, nil
	case key.Matches(msg, keyChoose):
		return m.chooseMode()
	}
	switch msg.String() {
	case "1":
		m.modeMenu = 0
		return m.chooseMode()
	case "2":
		m.modeMenu = 1
		return m.chooseMode()
	case "q", "esc", "ctrl+c":
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *wizard) chooseMode() (tea.Model, tea.Cmd) {
	m.modeChosen = true
	switch m.modeMenu {
	case 0:
		m.mode = ModeInstructions
		if m.flow.InstructionsError != nil {
			m.err = m.flow.InstructionsError
			return m, tea.Quit
		}
		if m.flow.Setup {
			m.step = stepSetup
		} else if len(m.flow.Harnesses) == 0 {
			m.cancelled = true
			return m, tea.Quit
		} else {
			m.step = stepSelect
		}
	case 1:
		m.mode = ModeSkills
		if m.flow.SkillsError != nil {
			m.err = m.flow.SkillsError
			return m, tea.Quit
		}
		if m.flow.Skills == nil || len(m.flow.Skills.Skills) == 0 || len(m.flow.Skills.Harnesses) == 0 {
			m.noChanges = true
			return m, tea.Quit
		}
		m.step = stepMatrix
	default:
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
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
	if m.step == stepSelect && len(m.flow.Harnesses) == 0 {
		m.cancelled = true
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

// updateMatrix drives the independent skill-by-harness matrix.
func (m *wizard) updateMatrix(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	matrix := m.flow.Skills
	if matrix == nil || len(matrix.Skills) == 0 || len(matrix.Harnesses) == 0 {
		m.cancelled = true
		return m, tea.Quit
	}
	if m.matrixRow >= len(matrix.Skills) {
		m.matrixRow = len(matrix.Skills) - 1
	}
	if m.matrixCol >= len(matrix.Harnesses) {
		m.matrixCol = len(matrix.Harnesses) - 1
	}

	switch {
	case key.Matches(msg, keyUpward):
		m.matrixRow = (m.matrixRow + len(matrix.Skills) - 1) % len(matrix.Skills)
	case key.Matches(msg, keyDownward):
		m.matrixRow = (m.matrixRow + 1) % len(matrix.Skills)
	case key.Matches(msg, keyLeftward):
		m.matrixCol = (m.matrixCol + len(matrix.Harnesses) - 1) % len(matrix.Harnesses)
	case key.Matches(msg, keyRightward):
		m.matrixCol = (m.matrixCol + 1) % len(matrix.Harnesses)
	case key.Matches(msg, keyToggle):
		m.toggleMatrixCell()
	case key.Matches(msg, keyAll):
		for key, cell := range matrix.Cells {
			if cell.Selectable() {
				m.matrixSelected[key] = true
			}
		}
	case key.Matches(msg, keyNone):
		m.matrixSelected = make(map[string]bool)
	case key.Matches(msg, keyReview):
		return m.reviewMatrix()
	case key.Matches(msg, keyCancel):
		m.cancelled = true
		return m, tea.Quit
	case key.Matches(msg, keyHelp):
		m.help.ShowAll = !m.help.ShowAll
	}
	return m, nil
}

func (m *wizard) toggleMatrixCell() {
	matrix := m.flow.Skills
	if matrix == nil || m.matrixRow < 0 || m.matrixRow >= len(matrix.Skills) ||
		m.matrixCol < 0 || m.matrixCol >= len(matrix.Harnesses) {
		return
	}
	cell, ok := matrix.Cell(matrix.Skills[m.matrixRow], matrix.Harnesses[m.matrixCol].ID)
	if !ok || !cell.Selectable() {
		return
	}
	if m.matrixSelected[cell.Key()] {
		delete(m.matrixSelected, cell.Key())
	} else {
		m.matrixSelected[cell.Key()] = true
	}
}

// reviewMatrix asks the caller to build the review for the current matrix
// selection; no changes at all ends the wizard instead.
func (m *wizard) reviewMatrix() (tea.Model, tea.Cmd) {
	if m.flow.SkillsReview == nil {
		m.noChanges = true
		return m, tea.Quit
	}
	review, ok, err := m.flow.SkillsReview(m.MatrixSelection())
	if err != nil {
		m.err = err
		return m, tea.Quit
	}
	if !ok {
		m.noChanges = true
		return m, tea.Quit
	}
	m.review = review
	m.reviewOffset = 0
	m.step = stepReview
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
	m.reviewOffset = 0
	m.step = stepReview
	return m, nil
}

func (m *wizard) updateReview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keyApply):
		m.applied = true
		return m, tea.Quit
	case key.Matches(msg, keyBack):
		if m.mode == ModeSkills {
			m.step = stepMatrix
		} else {
			m.step = stepSelect
		}
	case key.Matches(msg, keyQuit):
		m.cancelled = true
		return m, tea.Quit
	case key.Matches(msg, keyHelp):
		m.help.ShowAll = !m.help.ShowAll
	case key.Matches(msg, keyUpward):
		if m.reviewOffset > 0 {
			m.reviewOffset--
		}
	case key.Matches(msg, keyDownward):
		if m.reviewOffset < m.maxReviewOffset() {
			m.reviewOffset++
		}
	case key.Matches(msg, keyPageUp):
		m.reviewOffset -= m.reviewVisibleHeight() / 2
		if m.reviewOffset < 0 {
			m.reviewOffset = 0
		}
	case key.Matches(msg, keyPageDown):
		m.reviewOffset += m.reviewVisibleHeight() / 2
		if max := m.maxReviewOffset(); m.reviewOffset > max {
			m.reviewOffset = max
		}
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

func (m *wizard) reviewVisibleHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	visible := h - 13
	if visible < 1 {
		visible = 1
	}
	return visible
}

func (m *wizard) reviewLines() []string {
	_, inner := cardLayout(m.width)
	t := newTheme(m.dark)
	return strings.Split(t.plan.Width(inner).Render(m.review.Text), "\n")
}

func (m *wizard) maxReviewOffset() int {
	max := len(m.reviewLines()) - m.reviewVisibleHeight()
	if max < 0 {
		max = 0
	}
	return max
}

// reviewSections renders the scrollable plan body and the apply prompt.
func (m *wizard) reviewSections(t theme, inner int) []string {
	lines := m.reviewLines()
	visible := m.reviewVisibleHeight()
	offset := m.reviewOffset
	if max := len(lines) - visible; offset > max {
		offset = max
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + visible
	if end > len(lines) {
		end = len(lines)
	}

	sections := make([]string, 0, visible+4)
	sections = append(sections, lines[offset:end]...)
	if len(lines) > visible {
		sections = append(sections, "", t.faint.Render(fmt.Sprintf("lines %d-%d of %d", offset+1, end, len(lines))))
	}
	sections = append(sections, "", t.accent.Render("❯")+" "+t.bold.Render("Apply this plan?"))
	return sections
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
	case stepMode:
		subtitle = "What do you want to manage?"
		sections = m.modeSections(t, inner)
		helpText = t.helpLine(m.help, modeKeys{}, inner)
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
	case stepMatrix:
		subtitle = "Link your shared skills into each harness."
		sections = []string{
			m.renderSkillsMatrix(t, inner),
			"",
			t.faint.Render(truncateCell(m.matrixDetails(), inner)),
			m.matrixStatus(),
		}
		helpText = t.helpLine(m.help, matrixKeys{}, inner)
	default:
		subtitle = "Review and confirm."
		sections = m.reviewSections(t, inner)
		helpText = t.helpLine(m.help, reviewKeys{}, inner)
	}
	v := tea.NewView(t.card(pad, inner, subtitle, sections, helpText))
	v.AltScreen = true
	return v
}

// modeSections renders the launch menu.
func (m *wizard) modeSections(t theme, inner int) []string {
	lines := []string{
		lipgloss.NewStyle().Width(inner).Render("Instructions and skills live in one shared agents directory, but each harness gets its own links."),
		"",
	}
	for i, item := range modeMenuItems {
		if i == m.modeMenu {
			lines = append(lines, t.bar.Render(padRight("❯ "+item, inner)))
			continue
		}
		lines = append(lines, "  "+item)
	}
	return lines
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
