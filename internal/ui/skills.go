package ui

import (
	"fmt"
	"strings"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/skill"
)

// MatrixCell is one skill/harness cell in the skills picker.
type MatrixCell struct {
	Skill       string
	HarnessID   string
	HarnessName string
	Source      string
	Target      string
	State       harness.State
	// Unavailable explains why this pair cannot be linked, or is "".
	Unavailable string
}

// Key is the cell's stable identity.
func (c MatrixCell) Key() string { return skill.CellKey(c.Skill, c.HarnessID) }

// SkillsMatrix is the prepared data behind the skills picker.
type SkillsMatrix struct {
	Skills    []string
	Harnesses []harness.Harness
	Cells     map[string]MatrixCell
	Initial   map[string]bool
}

// Cell returns the cell for one skill and harness.
func (m SkillsMatrix) Cell(skillName, harnessID string) (MatrixCell, bool) {
	cell, ok := m.Cells[skill.CellKey(skillName, harnessID)]
	return cell, ok
}

// Selectable reports whether a cell can be toggled. Conflicts and statically
// unavailable pairs cannot, except that an already-managed reserved-name link
// stays selectable so it can be cleaned up.
func (c MatrixCell) Selectable() bool {
	if c.State == harness.Path || c.State == harness.Symlink {
		return false
	}
	return c.Unavailable == "" || c.State == harness.Managed
}

func matrixSymbol(cell MatrixCell, selected bool) string {
	switch {
	case cell.State == harness.Path || cell.State == harness.Symlink:
		return "!"
	case selected:
		return "✓"
	case cell.Unavailable != "":
		return "–"
	default:
		return "·"
	}
}

func compactHarnessName(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return name
	}
	first := fields[0]
	if cellWidth(first) > 8 {
		return truncateRight(first, 7) + "…"
	}
	return first
}

// renderSkillsMatrix draws the matrix step: a compact header per harness, one
// row per skill, markers for selection and conflicts, and a details line for
// the focused cell. Rows and columns scroll so every cell stays reachable.
func (m *wizard) renderSkillsMatrix(t theme, inner int) string {
	matrix := m.flow.Skills
	if matrix == nil || len(matrix.Skills) == 0 {
		return t.faint.Render("(no shared skills found)")
	}

	const (
		lead     = 3
		colWidth = 10
	)
	skillWidth := inner - lead - colWidth
	if skillWidth > 24 {
		skillWidth = 24
	}
	if skillWidth < 6 {
		skillWidth = 6
	}
	visibleCols := 1
	if inner > lead+skillWidth {
		visibleCols = (inner - lead - skillWidth) / colWidth
	}
	if visibleCols < 1 {
		visibleCols = 1
	}
	if visibleCols > len(matrix.Harnesses) {
		visibleCols = len(matrix.Harnesses)
	}

	left := 0
	if m.matrixCol < left {
		left = m.matrixCol
	}
	if m.matrixCol >= left+visibleCols {
		left = m.matrixCol - visibleCols + 1
	}
	if left < 0 {
		left = 0
	}
	if max := len(matrix.Harnesses) - visibleCols; left > max {
		left = max
	}
	if left < 0 {
		left = 0
	}

	visibleRows := m.matrixVisibleRows()
	top := 0
	if m.matrixRow < top {
		top = m.matrixRow
	}
	if m.matrixRow >= top+visibleRows {
		top = m.matrixRow - visibleRows + 1
	}
	if top < 0 {
		top = 0
	}
	if max := len(matrix.Skills) - visibleRows; top > max {
		top = max
	}
	if top < 0 {
		top = 0
	}

	header := strings.Repeat(" ", lead) + padRight("Skill", skillWidth)
	for col := left; col < left+visibleCols; col++ {
		name := padRight(compactHarnessName(matrix.Harnesses[col].Name), colWidth-1)
		if col == m.matrixCol {
			header += t.accent.Render(name) + " "
		} else {
			header += padRight(name, colWidth)
		}
	}
	lines := []string{t.bold.Render(header), t.faint.Render(strings.Repeat("─", minWidth(inner, lead+skillWidth+visibleCols*colWidth)))}

	for row := top; row < top+visibleRows && row < len(matrix.Skills); row++ {
		skill := matrix.Skills[row]
		cursor := " "
		if row == m.matrixRow {
			cursor = "❯"
		}
		line := cursor + "  " + padRight(truncateCell(skill, skillWidth), skillWidth)
		for col := left; col < left+visibleCols; col++ {
			cell, ok := matrix.Cell(skill, matrix.Harnesses[col].ID)
			if !ok {
				line += padRight(" ", colWidth)
				continue
			}
			symbol := matrixSymbol(cell, m.matrixSelected[cell.Key()])
			cellText := padRight(symbol, colWidth)
			switch {
			case col == m.matrixCol && row == m.matrixRow:
				cellText = t.accent.Render(padRight(symbol, colWidth))
			case symbol == "✓":
				cellText = t.green.Render(padRight(symbol, colWidth))
			case symbol == "!":
				cellText = t.danger.Render(padRight(symbol, colWidth))
			default:
				cellText = t.faint.Render(padRight(symbol, colWidth))
			}
			line += cellText
		}
		if row == m.matrixRow {
			line = t.bar.Render(padRight(line, inner))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *wizard) matrixVisibleRows() int {
	const fixedLines = 14
	h := m.height
	if h <= 0 {
		h = 24
	}
	visible := h - fixedLines
	if visible < 1 {
		visible = 1
	}
	return visible
}

// matrixDetails describes the focused cell for the details line.
func (m *wizard) matrixDetails() string {
	matrix := m.flow.Skills
	if matrix == nil || len(matrix.Skills) == 0 || len(matrix.Harnesses) == 0 {
		return ""
	}
	row := m.matrixRow
	col := m.matrixCol
	if row < 0 || row >= len(matrix.Skills) || col < 0 || col >= len(matrix.Harnesses) {
		return ""
	}
	skill := matrix.Skills[row]
	h := matrix.Harnesses[col]
	cell, ok := matrix.Cell(skill, h.ID)
	if !ok {
		return ""
	}
	target := harness.DisplayPath(cell.Target, m.flow.Ctx.Home)
	var state string
	switch {
	case cell.Unavailable != "" && cell.State != harness.Managed:
		state = cell.Unavailable
	case cell.State == harness.Path:
		state = "existing path; not overwritten"
	case cell.State == harness.Symlink:
		state = "unrelated symlink; not overwritten"
	case cell.State == harness.Managed && m.matrixSelected[cell.Key()]:
		state = "managed link; selected"
		if cell.Unavailable != "" {
			state += " (" + cell.Unavailable + ")"
		}
	case cell.State == harness.Managed:
		state = "managed link; will be removed"
	case m.matrixSelected[cell.Key()]:
		state = "will be created"
	default:
		state = "not linked"
	}
	return fmt.Sprintf("%s %s → %s · %s", skill, h.Name, target, state)
}

// matrixStatus summarizes desired selections and pending changes.
func (m *wizard) matrixStatus() string {
	matrix := m.flow.Skills
	if matrix == nil {
		return ""
	}
	total := len(matrix.Cells)
	selected, creates, removes, conflicts, unavailable := 0, 0, 0, 0, 0
	for key, cell := range matrix.Cells {
		if !cell.Selectable() {
			if cell.Unavailable != "" {
				unavailable++
			} else {
				conflicts++
			}
		}
		if m.matrixSelected[key] {
			selected++
		}
		switch {
		case m.matrixSelected[key] && cell.State == harness.Absent && cell.Unavailable == "":
			creates++
		case !m.matrixSelected[key] && cell.State == harness.Managed:
			removes++
		}
	}
	parts := []string{
		fmt.Sprintf("Selected %d of %d", selected, total),
		fmt.Sprintf("create %d", creates),
		fmt.Sprintf("remove %d", removes),
	}
	if conflicts > 0 {
		parts = append(parts, fmt.Sprintf("conflicts %d", conflicts))
	}
	if unavailable > 0 {
		parts = append(parts, fmt.Sprintf("unavailable %d", unavailable))
	}
	return strings.Join(parts, " · ")
}

// RenderSkillsTable renders the read-only skills matrix without a trailing
// newline. selected holds the managed cells.
func RenderSkillsTable(matrix SkillsMatrix, ctx harness.Context, selected map[string]bool) (string, error) {
	headers := []string{"", "Skill"}
	for _, h := range matrix.Harnesses {
		headers = append(headers, h.Name)
	}
	rows := make([][]string, 0, len(matrix.Skills))
	for _, skill := range matrix.Skills {
		row := []string{"", skill}
		for _, h := range matrix.Harnesses {
			cell, ok := matrix.Cell(skill, h.ID)
			if !ok {
				row = append(row, "")
				continue
			}
			row = append(row, matrixSymbol(cell, selected[cell.Key()]))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		empty := []string{"", "(no shared skills)"}
		for range matrix.Harnesses {
			empty = append(empty, "")
		}
		rows = append(rows, empty)
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = cellWidth(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if width := cellWidth(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}

	border := func(left, middle, right string) string {
		segments := make([]string, len(widths))
		for i, width := range widths {
			segments[i] = strings.Repeat("─", width+2)
		}
		return left + strings.Join(segments, middle) + right
	}
	renderRow := func(cells []string) string {
		padded := make([]string, len(cells))
		for i, cell := range cells {
			padded[i] = cell + strings.Repeat(" ", widths[i]-cellWidth(cell))
		}
		return "│ " + strings.Join(padded, " │ ") + " │"
	}

	lines := []string{
		"Shared skills: " + ctx.SourceSkillsDir,
		"",
		border("┌", "┬", "┐"),
		renderRow(headers),
		border("├", "┼", "┤"),
	}
	for _, row := range rows {
		lines = append(lines, renderRow(row))
	}
	lines = append(lines, border("└", "┴", "┘"))
	lines = append(lines, "Legend: ✓ managed  · absent  ! conflict  – unavailable")

	var notes []string
	for _, skill := range matrix.Skills {
		for _, h := range matrix.Harnesses {
			cell, ok := matrix.Cell(skill, h.ID)
			if !ok {
				continue
			}
			switch {
			case cell.Unavailable != "":
				notes = append(notes, fmt.Sprintf("  %s: %s",
					harness.DisplayPath(cell.Target, ctx.Home), cell.Unavailable))
			case cell.State == harness.Path:
				notes = append(notes, fmt.Sprintf("  %s: existing path; not overwritten",
					harness.DisplayPath(cell.Target, ctx.Home)))
			case cell.State == harness.Symlink:
				notes = append(notes, fmt.Sprintf("  %s: unrelated symlink; not overwritten",
					harness.DisplayPath(cell.Target, ctx.Home)))
			}
		}
	}
	if len(notes) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Notes:")
		lines = append(lines, notes...)
	}
	return strings.Join(lines, "\n"), nil
}

func minWidth(a, b int) int {
	if a < b {
		return a
	}
	return b
}
