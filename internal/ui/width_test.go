package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

func TestCellWidth(t *testing.T) {
	cases := []struct {
		name string
		cell string
		want int
	}{
		{name: "empty", cell: "", want: 0},
		{name: "ascii", cell: "Harness", want: 7},
		{name: "cjk", cell: "会議", want: 4},
		{name: "cjk mixed with ascii", cell: "会a", want: 3},
		{name: "combining mark", cell: "e\u0301", want: 1},
		{name: "emoji", cell: "🙂", want: 2},
		{name: "emoji zwj sequence", cell: "👩‍👩‍👧‍👦", want: 2},
		{name: "ansi styled", cell: "\x1b[31mred\x1b[0m", want: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cellWidth(tc.cell); got != tc.want {
				t.Errorf("cellWidth(%q) = %d, want %d", tc.cell, got, tc.want)
			}
		})
	}
}

func TestTruncateCell(t *testing.T) {
	cases := []struct {
		name  string
		cell  string
		width int
		want  string
	}{
		{name: "empty", cell: "", width: 5, want: ""},
		{name: "zero width", cell: "alpha", width: 0, want: ""},
		{name: "one column", cell: "alpha", width: 1, want: "…"},
		{name: "ascii unchanged", cell: "alpha", width: 5, want: "alpha"},
		{name: "ascii keeps ends", cell: "abcdefghij", width: 7, want: "abc…hij"},
		{name: "ascii even width", cell: "abcdefghij", width: 8, want: "abcd…hij"},
		{name: "cjk keeps whole characters", cell: "会議室の予定", width: 7},
		{name: "emoji zwj in the head", cell: "👩‍👩‍👧‍👦family", width: 5, want: "👩‍👩‍👧‍👦…ly"},
		{name: "emoji zwj in the tail", cell: "family👩‍👩‍👧‍👦", width: 5, want: "fa…👩‍👩‍👧‍👦"},
		{name: "combining marks stay attached", cell: "cafe\u0301cafe\u0301cafe\u0301", width: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateCell(tc.cell, tc.width)
			if tc.want != "" && got != tc.want {
				t.Fatalf("truncateCell(%q, %d) = %q, want %q", tc.cell, tc.width, got, tc.want)
			}
			if width := lipgloss.Width(got); width > tc.width {
				t.Errorf("truncateCell(%q, %d) is %d columns wide", tc.cell, tc.width, width)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateCell(%q, %d) = %q is not valid UTF-8", tc.cell, tc.width, got)
			}
			if tc.width > 1 && cellWidth(tc.cell) > tc.width {
				head, tail, ok := strings.Cut(got, "…")
				if !ok {
					t.Fatalf("truncateCell(%q, %d) = %q has no ellipsis", tc.cell, tc.width, got)
				}
				if !strings.HasPrefix(tc.cell, head) || !strings.HasSuffix(tc.cell, tail) {
					t.Errorf("truncateCell(%q, %d) = %q does not keep both ends", tc.cell, tc.width, got)
				}
				if strings.HasPrefix(got, "\u200d") || strings.HasSuffix(got, "\u200d") {
					t.Errorf("truncateCell(%q, %d) = %q split a ZWJ sequence", tc.cell, tc.width, got)
				}
			}
		})
	}
}

func TestCompactHarnessName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{name: "short name", want: "Claude"},
		{name: "eight columns", want: "OpenCode"},
		{name: "long name", want: "Antigra…"},
		{name: "empty", want: ""},
	}
	inputs := []string{"Claude Code", "OpenCode", "Antigravity", ""}
	for i, tc := range cases {
		if got := compactHarnessName(inputs[i]); got != tc.want {
			t.Errorf("compactHarnessName(%q) = %q, want %q", inputs[i], got, tc.want)
		}
	}

	for _, name := range []string{"日本語の長い名前", "🙂🙂🙂🙂🙂"} {
		got := compactHarnessName(name)
		if width := lipgloss.Width(got); width > 8 {
			t.Errorf("compactHarnessName(%q) = %q is %d columns wide", name, got, width)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("compactHarnessName(%q) = %q, want an ellipsis", name, got)
		}
	}
}

func TestSkillsTableWideCharactersAlign(t *testing.T) {
	ctx := testContext(t)
	h := harness.Harness{
		ID:        "cjk",
		Name:      "日本語ツール",
		Target:    filepath.Join(ctx.Home, "cjk", "AGENTS.md"),
		SkillsDir: filepath.Join(ctx.Home, "cjk", "skills"),
	}
	matrix := SkillsMatrix{
		Skills:    []string{"日本語スキル", "alpha"},
		Harnesses: []harness.Harness{h},
		Cells:     make(map[string]MatrixCell),
	}
	for _, skill := range matrix.Skills {
		cell := MatrixCell{
			Skill:       skill,
			HarnessID:   h.ID,
			HarnessName: h.Name,
			Source:      filepath.Join(ctx.SourceSkillsDir, skill),
			Target:      filepath.Join(h.SkillsDir, skill),
			State:       harness.Absent,
		}
		matrix.Cells[cell.Key()] = cell
	}

	table, err := RenderSkillsTable(matrix, ctx, nil)
	if err != nil {
		t.Fatalf("RenderSkillsTable: %v", err)
	}
	assertTableLinesAligned(t, table)
	if !strings.Contains(table, "日本語スキル") {
		t.Errorf("table lost the wide skill name:\n%s", table)
	}
}

func TestRenderTableWideCharactersAlign(t *testing.T) {
	ctx := testContext(t)
	harnesses := []harness.Harness{{
		ID:          "cjk",
		Name:        "日本語ツール",
		Target:      filepath.Join(ctx.Home, "会議", "AGENTS.md"),
		Executables: []string{"missing-cjk"},
	}}

	table, err := RenderTable(harnesses, ctx, nil, 0)
	if err != nil {
		t.Fatalf("RenderTable: %v", err)
	}
	assertTableLinesAligned(t, table)
}

// assertTableLinesAligned checks that every box-drawing line of a rendered
// table occupies the same number of terminal columns.
func assertTableLinesAligned(t *testing.T, table string) {
	t.Helper()
	width := -1
	lines := 0
	for _, line := range strings.Split(table, "\n") {
		if line == "" || !strings.ContainsRune("│┌├└", []rune(line)[0]) {
			continue
		}
		lines++
		got := lipgloss.Width(line)
		if width < 0 {
			width = got
			continue
		}
		if got != width {
			t.Errorf("table line %q is %d columns wide, want %d", line, got, width)
		}
	}
	if lines < 4 {
		t.Errorf("expected a table, found %d box-drawing lines:\n%s", lines, table)
	}
}
