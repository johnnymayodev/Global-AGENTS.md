package plan

import (
	"fmt"
	"io"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/skill"
)

// SkillCell is one skill/harness pair in a skills plan.
type SkillCell struct {
	Skill       string
	HarnessID   string
	HarnessName string
	Source      string
	Target      string
	// Unavailable explains why this pair cannot be linked, or is "".
	Unavailable string
}

// Key is the cell's stable identity across the UI and plan packages.
func (c SkillCell) Key() string { return skill.CellKey(c.Skill, c.HarnessID) }

// SkillsPlan is the planned work for a set of skill/harness cells.
type SkillsPlan struct {
	ToCreate      []SkillCell
	ToRemove      []SkillCell
	AlreadyLinked []SkillCell
	AlreadyAbsent []SkillCell
	Unmanaged     []SkillCell
	Unavailable   []SkillCell
}

// SkillLinks plans symlink creation for every available cell. It refuses to
// replace unrelated symlinks or existing paths.
func SkillLinks(cells []SkillCell) (SkillsPlan, error) {
	return planSkills(cells, nil, skillLinkAll)
}

// SkillUnlinks plans removal of managed symlinks for every cell. Anything that
// is not a managed symlink is left untouched, and the sources need not exist.
func SkillUnlinks(cells []SkillCell) (SkillsPlan, error) {
	return planSkills(cells, nil, skillUnlinkAll)
}

// SkillReconcile builds the plan for a matrix selection: selected cells are
// linked, deselected managed cells are removed, and conflicts stay untouched.
func SkillReconcile(cells []SkillCell, selected map[string]bool) (SkillsPlan, error) {
	return planSkills(cells, selected, skillReconcile)
}

type skillPlanMode int

const (
	skillLinkAll skillPlanMode = iota
	skillUnlinkAll
	skillReconcile
)

func planSkills(cells []SkillCell, selected map[string]bool, mode skillPlanMode) (SkillsPlan, error) {
	plan := SkillsPlan{}

	// Deduplicate cells by identity, keeping the first occurrence.
	seenCells := make(map[string]bool, len(cells))
	unique := make([]SkillCell, 0, len(cells))
	for _, cell := range cells {
		key := cell.Key()
		if seenCells[key] {
			continue
		}
		seenCells[key] = true
		unique = append(unique, cell)
	}

	// One destination may not be claimed by two different sources.
	selectedTargets := make(map[string]string)
	for _, cell := range unique {
		if !cellSelected(cell, selected, mode) {
			continue
		}
		if cell.Unavailable != "" && mode != skillReconcile {
			continue
		}
		if source, ok := selectedTargets[cell.Target]; ok && source != cell.Source {
			return plan, fmt.Errorf("refusing to point one destination at different skills: %s; no changes made", cell.Target)
		}
		selectedTargets[cell.Target] = cell.Source
	}

	seenCreate := make(map[string]bool)
	seenRemove := make(map[string]bool)
	seenLinked := make(map[string]bool)
	seenAbsent := make(map[string]bool)
	seenUnmanaged := make(map[string]bool)

	appendOnce := func(dst *[]SkillCell, seen map[string]bool, cell SkillCell) {
		key := cell.Target + "\x00" + cell.Source
		if seen[key] {
			return
		}
		seen[key] = true
		*dst = append(*dst, cell)
	}

	for _, cell := range unique {
		state, err := harness.TargetState(cell.Target, cell.Source)
		if err != nil {
			return plan, err
		}
		selectedHere := cellSelected(cell, selected, mode)

		if cell.Unavailable != "" {
			// An already-managed link stays visible for cleanup. Unlink
			// ignores availability entirely.
			if state != harness.Managed && mode != skillUnlinkAll {
				plan.Unavailable = append(plan.Unavailable, cell)
				continue
			}
		}

		switch mode {
		case skillLinkAll:
			switch state {
			case harness.Managed:
				appendOnce(&plan.AlreadyLinked, seenLinked, cell)
			case harness.Absent:
				appendOnce(&plan.ToCreate, seenCreate, cell)
			case harness.Symlink:
				return plan, fmt.Errorf("refusing to replace unrelated symlink: %s; no changes made", cell.Target)
			default:
				return plan, fmt.Errorf("refusing to replace existing path: %s; no changes made", cell.Target)
			}
		case skillUnlinkAll:
			switch state {
			case harness.Managed:
				appendOnce(&plan.ToRemove, seenRemove, cell)
			case harness.Absent:
				appendOnce(&plan.AlreadyAbsent, seenAbsent, cell)
			default:
				appendOnce(&plan.Unmanaged, seenUnmanaged, cell)
			}
		default:
			if selectedHere {
				switch state {
				case harness.Managed:
					appendOnce(&plan.AlreadyLinked, seenLinked, cell)
				case harness.Absent:
					appendOnce(&plan.ToCreate, seenCreate, cell)
				default:
					appendOnce(&plan.Unmanaged, seenUnmanaged, cell)
				}
				continue
			}
			switch state {
			case harness.Managed:
				if _, claimed := selectedTargets[cell.Target]; claimed {
					appendOnce(&plan.AlreadyLinked, seenLinked, cell)
					continue
				}
				appendOnce(&plan.ToRemove, seenRemove, cell)
			case harness.Absent:
				appendOnce(&plan.AlreadyAbsent, seenAbsent, cell)
			default:
				appendOnce(&plan.Unmanaged, seenUnmanaged, cell)
			}
		}
	}

	return plan, nil
}

func cellSelected(cell SkillCell, selected map[string]bool, mode skillPlanMode) bool {
	switch mode {
	case skillLinkAll:
		return true
	case skillUnlinkAll:
		return false
	default:
		return selected[cell.Key()]
	}
}

// CreateSkillLinks creates the given symlinks, logging progress to stdout. On
// failure it logs the stop position to stderr and returns the error.
func CreateSkillLinks(cells []SkillCell, ctx harness.Context, stdout, stderr io.Writer) error {
	items := make([]linkTarget, 0, len(cells))
	for _, cell := range cells {
		items = append(items, linkTarget{Source: cell.Source, Target: cell.Target, Directory: true})
	}
	return createTargets(items, ctx, stdout, stderr)
}

// RemoveManagedSkillLinks removes the given managed symlinks, logging progress
// to stdout. Entries that are no longer managed symlinks are left untouched.
func RemoveManagedSkillLinks(cells []SkillCell, ctx harness.Context, stdout, stderr io.Writer) error {
	items := make([]linkTarget, 0, len(cells))
	for _, cell := range cells {
		items = append(items, linkTarget{Source: cell.Source, Target: cell.Target})
	}
	return removeTargets(items, ctx, stdout, stderr)
}

// ApplySkills applies a skills plan, creating links before removing any
// managed link so a failed addition cannot remove an existing link.
func ApplySkills(p SkillsPlan, ctx harness.Context, stdout, stderr io.Writer) error {
	if err := CreateSkillLinks(p.ToCreate, ctx, stdout, stderr); err != nil {
		if len(p.ToRemove) > 0 {
			fmt.Fprintln(stderr, "No planned managed links were removed because link creation did not finish.")
		}
		return err
	}
	return RemoveManagedSkillLinks(p.ToRemove, ctx, stdout, stderr)
}
