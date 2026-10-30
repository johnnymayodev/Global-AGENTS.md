package ui

import (
	"strings"
	"unicode"

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
const chipText = "agent-links"

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
// the file name stays readable. It cuts only between grapheme clusters, so a
// combining mark, emoji modifier, or ZWJ sequence is never split, and the
// result never exceeds width columns.
func truncateCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	clusters := graphemeClusters(s)
	total := 0
	for _, cluster := range clusters {
		total += cellWidth(cluster)
	}
	if total <= width {
		return s
	}
	if width == 1 {
		return "…"
	}

	tailBudget := (width - 1) / 2
	headBudget := width - 1 - tailBudget

	headEnd := 0
	used := 0
	for headEnd < len(clusters) {
		w := cellWidth(clusters[headEnd])
		if used+w > headBudget {
			break
		}
		used += w
		headEnd++
	}
	tailStart := len(clusters)
	used = 0
	for tailStart > headEnd {
		w := cellWidth(clusters[tailStart-1])
		if used+w > tailBudget {
			break
		}
		used += w
		tailStart--
	}
	return strings.Join(clusters[:headEnd], "") + "…" + strings.Join(clusters[tailStart:], "")
}

// truncateRight returns the longest prefix of s that fits in width columns,
// cutting only between grapheme clusters.
func truncateRight(s string, width int) string {
	var b strings.Builder
	used := 0
	for _, cluster := range graphemeClusters(s) {
		w := cellWidth(cluster)
		if used+w > width {
			break
		}
		b.WriteString(cluster)
		used += w
	}
	return b.String()
}

// graphemeClusters splits s into the text elements a terminal draws as one
// unit. It covers what table cells need to avoid splitting: combining marks,
// variation selectors, emoji modifiers, keycaps, tag characters, regional
// indicator pairs, and ZWJ sequences.
func graphemeClusters(s string) []string {
	var clusters []string
	var prev rune
	regional := 0
	for _, r := range s {
		join := len(clusters) > 0 && continuesCluster(prev, r, regional)
		if join {
			clusters[len(clusters)-1] += string(r)
		} else {
			clusters = append(clusters, string(r))
		}
		if isRegionalIndicator(r) {
			if join {
				regional++
			} else {
				regional = 1
			}
		} else {
			regional = 0
		}
		prev = r
	}
	return clusters
}

// continuesCluster reports whether r extends the cluster ending at prev.
func continuesCluster(prev, r rune, regional int) bool {
	switch {
	case prev == '\u200d' || r == '\u200d':
		return true
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Mc, r) || unicode.Is(unicode.Me, r):
		return true
	case r >= 0xfe00 && r <= 0xfe0f: // variation selectors
		return true
	case r >= 0xe0100 && r <= 0xe01ef: // variation selectors supplement
		return true
	case r >= 0x1f3fb && r <= 0x1f3ff: // emoji modifiers
		return true
	case r >= 0xe0020 && r <= 0xe007f: // tag characters
		return true
	case r == 0x20e3: // combining enclosing keycap
		return true
	case isRegionalIndicator(prev) && isRegionalIndicator(r) && regional%2 == 1:
		return true
	}
	return false
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1f1e6 && r <= 0x1f1ff
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
