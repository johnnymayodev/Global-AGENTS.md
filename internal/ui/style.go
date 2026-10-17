package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// The interactive screens share one look, in the spirit of the Bubble Tea
// examples: a rounded card like list-fancy, the table example's bold header
// and full-row cursor bar, the views example's per-row badges, and the help
// example's key hints. Only the TUI draws these styles — the plain path in
// plain.go stays byte-for-byte stable for scripts and the golden tests.
//
// Colors adapt to the terminal background reported by tea.BackgroundColorMsg.
// Until the terminal answers, models keep the dark palette, the safer default.

// chipText is the card's title chip.
const chipText = "Global AGENTS.md"

// Card geometry: the frame hugs a fixed width so every step lines up, capped
// on wide terminals and padded tighter when the window gets narrow.
const (
	maxCardWidth = 90
	cardMarginX  = 2
)

// cardLayout returns the frame's inner padding and content width for a
// terminal width.
func cardLayout(termWidth int) (pad, inner int) {
	card := termWidth - 2*cardMarginX
	if card > maxCardWidth {
		card = maxCardWidth
	}
	if card < 24 {
		card = 24
	}
	pad = 3
	if card < 64 {
		pad = 1
	}
	return pad, card - 2 - 2*pad
}

// theme carries the styles for one terminal background (light or dark).
type theme struct {
	accent lipgloss.Style // pink: cursor glyphs, prompts, the plan bar
	faint  lipgloss.Style // gray: borders, paths, hints
	bold   lipgloss.Style // emphasis in the terminal's own foreground
	green  lipgloss.Style
	danger lipgloss.Style
	chip   lipgloss.Style // white on Charm purple
	bar    lipgloss.Style // the cursor row
	frame  lipgloss.Style
	plan   lipgloss.Style
}

func newTheme(dark bool) theme {
	ld := lipgloss.LightDark(dark)
	var (
		accent = ld(lipgloss.Color("162"), lipgloss.Color("212"))
		green  = ld(lipgloss.Color("28"), lipgloss.Color("84"))
		faint  = ld(lipgloss.Color("245"), lipgloss.Color("241"))
		danger = ld(lipgloss.Color("160"), lipgloss.Color("203"))
	)
	purple := lipgloss.Color("#7D56F4")
	return theme{
		accent: lipgloss.NewStyle().Foreground(accent),
		faint:  lipgloss.NewStyle().Foreground(faint),
		bold:   lipgloss.NewStyle().Bold(true),
		green:  lipgloss.NewStyle().Foreground(green),
		danger: lipgloss.NewStyle().Foreground(danger),
		chip:   lipgloss.NewStyle().Background(purple).Foreground(lipgloss.Color("#FFFDF5")).Bold(true).Padding(0, 1),
		bar:    lipgloss.NewStyle().Background(purple).Foreground(lipgloss.Color("#FFFFFF")),
		frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(faint).
			Margin(1, cardMarginX, 1, cardMarginX),
		plan: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), false, false, false, true).
			BorderForeground(accent).
			PaddingLeft(1),
	}
}

// card renders the shared full-screen frame around one step: the title chip,
// a step subtitle, the step's sections, a divider, and the help line.
func (t theme) card(pad, inner int, subtitle string, sections []string, helpLine string) string {
	lines := []string{"", t.chip.Render(chipText), t.faint.Render(subtitle), ""}
	lines = append(lines, sections...)
	lines = append(lines, "", t.rule(inner), helpLine)
	return t.frame.Padding(0, pad).Render(strings.Join(lines, "\n"))
}

// rule is a faint horizontal divider across the card's content width.
func (t theme) rule(inner int) string {
	return t.faint.Render(strings.Repeat("─", inner))
}

// helpLine renders a step's key help, wrapped to the card's content width.
func (t theme) helpLine(model help.Model, keys help.KeyMap, inner int) string {
	model.SetWidth(inner)
	return model.View(keys)
}

// applyInputStyles paints the textinput prompt with the theme accent while
// keeping the stock focused/blurred behavior and cursor.
func applyInputStyles(input *textinput.Model, dark bool) {
	t := newTheme(dark)
	styles := textinput.DefaultStyles(dark)
	styles.Focused.Prompt = t.accent
	styles.Blurred.Prompt = t.accent
	input.SetStyles(styles)
}

// applyHelpStyles paints the key help: keys bold, descriptions and the
// separators faint.
func applyHelpStyles(model *help.Model, dark bool) {
	t := newTheme(dark)
	styles := help.DefaultStyles(dark)
	styles.ShortKey = t.bold
	styles.ShortDesc = t.faint
	styles.ShortSeparator = t.faint
	styles.FullKey = t.bold
	styles.FullDesc = t.faint
	styles.FullSeparator = t.faint
	styles.Ellipsis = t.faint
	model.Styles = styles
}

// padRight right-pads a rendered cell with plain spaces to width columns.
func padRight(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// truncateCell shortens a table cell to width columns, keeping both ends so
// the file name stays readable.
func truncateCell(s string, width int) string {
	runes := []rune(s)
	if width <= 1 || len(runes) <= width {
		return s
	}
	tail := (width - 1) / 2
	head := width - 1 - tail
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

// renderHarnessTable draws the select step's table: a bold header, a faint
// rule, the ❯ cursor column, a green ✓ for selected harnesses, installed /
// not found badges, and the linked path. The row under the cursor becomes
// one purple bar, following the table example's cursor treatment. Style
// sequences wrap whole cells, so plain substrings (harness names, badges)
// stay contiguous in the view.
func renderHarnessTable(t theme, harnesses []harness.Harness, ctx harness.Context, selected map[string]bool, cursor, inner int) (string, error) {
	type row struct {
		h         harness.Harness
		installed bool
		selected  bool
		path      string
	}
	rows := make([]row, 0, len(harnesses))
	for _, h := range harnesses {
		installed, err := harness.IsInstalled(h, ctx)
		if err != nil {
			return "", err
		}
		rows = append(rows, row{
			h:         h,
			installed: installed,
			selected:  selected[h.ID],
			path:      harness.DisplayPath(h.Target, ctx.Home),
		})
	}

	nameWidth := cellWidth("Harness")
	for _, r := range rows {
		if width := cellWidth(r.h.Name); width > nameWidth {
			nameWidth = width
		}
	}
	const (
		lead           = 5  // "❯ ✓  "
		installedWidth = 11 // "● installed" and "○ not found"
		minPathWidth   = 12
	)
	showInstalled := inner >= lead+nameWidth+1+installedWidth
	showPath := false
	pathWidth := 0
	if showInstalled {
		pathWidth = inner - (lead + nameWidth + 1 + installedWidth + 1)
		showPath = pathWidth >= minPathWidth
	}
	tableWidth := lead + nameWidth
	if showInstalled {
		tableWidth += 1 + installedWidth
	}
	if showPath {
		tableWidth += 1 + pathWidth
	}

	headers := strings.Repeat(" ", lead) + padRight("Harness", nameWidth)
	if showInstalled {
		headers += " " + padRight("Installed", installedWidth)
	}
	if showPath {
		headers += " Global instructions"
	}
	lines := []string{
		t.bold.Render(headers),
		t.faint.Render(strings.Repeat("─", tableWidth)),
	}
	if len(rows) == 0 {
		lines = append(lines, t.faint.Render("(none detected)"))
		return strings.Join(lines, "\n"), nil
	}

	for i, r := range rows {
		cursorMark := " "
		if i == cursor {
			cursorMark = "❯"
		}
		mark := " "
		if r.selected {
			mark = "✓"
		}
		badge := "○ not found"
		badgeStyled := t.faint.Render(badge)
		if r.installed {
			badge = "● installed"
			badgeStyled = t.green.Render(badge)
		}
		pathText := ""
		if showPath {
			pathText = truncateCell(r.path, pathWidth)
		}

		if i == cursor {
			line := cursorMark + " " + mark + "  " + padRight(r.h.Name, nameWidth)
			if showInstalled {
				line += " " + badge
			}
			if showPath {
				line += " " + pathText
			}
			lines = append(lines, t.bar.Render(padRight(line, inner)))
			continue
		}

		markCell := " "
		if r.selected {
			markCell = t.green.Render("✓")
		}
		line := cursorMark + " " + markCell + "  " + padRight(r.h.Name, nameWidth)
		if showInstalled {
			line += " " + padRight(badgeStyled, installedWidth)
		}
		if showPath {
			line += " " + t.faint.Render(pathText)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}
