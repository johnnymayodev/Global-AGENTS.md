package cli

import (
	"testing"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/ui"
)

func TestMatrixPlanCellsPreservesUnavailable(t *testing.T) {
	h := harness.Harness{ID: "claude", Name: "Claude Code", SkillsDir: "/home/.claude/skills"}
	cell := ui.MatrixCell{
		Skill:       "synced",
		HarnessID:   h.ID,
		HarnessName: h.Name,
		Source:      "/home/.global-agents/skills/synced",
		Target:      "/home/.claude/skills/synced",
		State:       harness.Absent,
		Unavailable: "matrix-only policy",
	}
	matrix := ui.SkillsMatrix{
		Skills:    []string{cell.Skill},
		Harnesses: []harness.Harness{h},
		Cells:     map[string]ui.MatrixCell{cell.Key(): cell},
	}

	cells := matrixPlanCells(matrix)
	if len(cells) != 1 {
		t.Fatalf("matrixPlanCells returned %d cells, want 1", len(cells))
	}
	if got := cells[0].Unavailable; got != cell.Unavailable {
		t.Fatalf("matrixPlanCells Unavailable = %q, want the cell's %q", got, cell.Unavailable)
	}
}
