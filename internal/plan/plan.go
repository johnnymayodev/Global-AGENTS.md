// Package plan builds and applies filesystem plans for linking the canonical
// instructions file and canonical skill directories into harness targets.
package plan

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// LinkPlan is the planned linking work for a set of harnesses.
type LinkPlan struct {
	ToCreate      []harness.Harness
	AlreadyLinked []harness.Harness
}

// UnlinkPlan is the planned unlink work for a set of harnesses.
type UnlinkPlan struct {
	ToRemove      []harness.Harness
	AlreadyAbsent []harness.Harness
	Untouched     []harness.Harness
}

// ReconcilePlan combines the linking and unlinking work of an interactive
// selection.
type ReconcilePlan struct {
	LinkPlan
	UnlinkPlan
}

// Links plans symlink creation for targets. It refuses to replace unrelated
// symlinks or existing paths and requires the canonical file to be present.
func Links(targets []harness.Harness, ctx harness.Context) (LinkPlan, error) {
	plan := LinkPlan{}
	if len(targets) == 0 {
		return plan, nil
	}
	if err := harness.AssertSourceFile(ctx); err != nil {
		return plan, err
	}

	seen := make(map[string]bool)
	for _, h := range targets {
		if seen[h.Target] {
			continue
		}
		seen[h.Target] = true

		state, err := harness.TargetState(h.Target, ctx.SourceFile)
		if err != nil {
			return plan, err
		}
		switch state {
		case harness.Managed:
			plan.AlreadyLinked = append(plan.AlreadyLinked, h)
		case harness.Absent:
			plan.ToCreate = append(plan.ToCreate, h)
		case harness.Symlink:
			return plan, fmt.Errorf("refusing to replace unrelated symlink: %s; no changes made", h.Target)
		default:
			return plan, fmt.Errorf("refusing to replace existing path: %s; no changes made", h.Target)
		}
	}

	return plan, nil
}

// Unlinks plans removal of managed symlinks for targets. Anything that is not
// a managed symlink is left untouched.
func Unlinks(targets []harness.Harness, ctx harness.Context) (UnlinkPlan, error) {
	plan := UnlinkPlan{}

	seen := make(map[string]bool)
	for _, h := range targets {
		if seen[h.Target] {
			continue
		}
		seen[h.Target] = true

		state, err := harness.TargetState(h.Target, ctx.SourceFile)
		if err != nil {
			return plan, err
		}
		switch state {
		case harness.Managed:
			plan.ToRemove = append(plan.ToRemove, h)
		case harness.Absent:
			plan.AlreadyAbsent = append(plan.AlreadyAbsent, h)
		default:
			plan.Untouched = append(plan.Untouched, h)
		}
	}

	return plan, nil
}

// Reconcile builds the plan for an interactive selection: the selected
// harnesses are linked and the deselected ones are unlinked, except that a
// target still claimed by a selected harness is never unlinked.
func Reconcile(visible []harness.Harness, selected map[string]bool, ctx harness.Context) (ReconcilePlan, error) {
	var chosen []harness.Harness
	chosenTargets := make(map[string]bool)
	for _, h := range visible {
		if selected[h.ID] {
			chosen = append(chosen, h)
			chosenTargets[h.Target] = true
		}
	}

	var deselected []harness.Harness
	for _, h := range visible {
		if !selected[h.ID] && !chosenTargets[h.Target] {
			deselected = append(deselected, h)
		}
	}

	linkPlan, err := Links(chosen, ctx)
	if err != nil {
		return ReconcilePlan{}, err
	}
	unlinkPlan, err := Unlinks(deselected, ctx)
	if err != nil {
		return ReconcilePlan{}, err
	}
	return ReconcilePlan{LinkPlan: linkPlan, UnlinkPlan: unlinkPlan}, nil
}

// linkTarget is one explicit source-to-destination filesystem mutation.
type linkTarget struct {
	Source string
	Target string
}

// CreateLinks creates the given symlinks, logging progress to stdout. On
// failure it logs the stop position to stderr and returns the error.
func CreateLinks(targets []harness.Harness, ctx harness.Context, stdout, stderr io.Writer) error {
	items := make([]linkTarget, 0, len(targets))
	for _, h := range targets {
		items = append(items, linkTarget{Source: ctx.SourceFile, Target: h.Target})
	}
	return createTargets(items, ctx, stdout, stderr)
}

// createTargets creates every item, stopping at the first failure.
func createTargets(items []linkTarget, ctx harness.Context, stdout, stderr io.Writer) error {
	for _, item := range items {
		directoryCreationStarted, err := createTarget(item, ctx, stdout)
		if err != nil {
			partial := ""
			if directoryCreationStarted {
				partial = " directory creation for this destination may be partial,"
			}
			fmt.Fprintf(stderr, "stopped at %s; earlier reported changes remain,%s and later destinations were not attempted.\n", item.Target, partial)
			return err
		}
	}
	return nil
}

// createTarget links one explicit source to one destination and reports
// whether parent-directory creation for the destination had started before
// any failure.
func createTarget(item linkTarget, ctx harness.Context, stdout io.Writer) (directoryCreationStarted bool, err error) {
	state, err := harness.TargetState(item.Target, item.Source)
	if err != nil {
		return false, err
	}
	if state == harness.Managed {
		fmt.Fprintf(stdout, "already linked %s\n", harness.DisplayPath(item.Target, ctx.Home))
		return false, nil
	}
	if state != harness.Absent {
		return false, fmt.Errorf("destination changed after planning; refusing to overwrite: %s", item.Target)
	}

	createdDir, err := mkdirParents(filepath.Dir(item.Target))
	if err != nil {
		return true, err
	}
	if createdDir != "" {
		fmt.Fprintf(stdout, "created directory %s\n", harness.DisplayPath(createdDir, ctx.Home))
	}
	if err := createSymlink(item.Source, item.Target); err != nil {
		return true, err
	}
	fmt.Fprintf(stdout, "created symlink %s\n", harness.DisplayPath(item.Target, ctx.Home))
	return true, nil
}

// createSymlink creates an exclusive symlink, translating EEXIST into the
// post-plan destination guard message.
func createSymlink(sourceFile, target string) error {
	err := os.Symlink(sourceFile, target)
	if err != nil && errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("destination appeared after planning; refusing to overwrite: %s", target)
	}
	return err
}

// mkdirParents creates dir and any missing parents, like
// fs.mkdirSync(dir, {recursive: true}). It returns the first directory path
// that was created, or "" when dir already existed.
func mkdirParents(dir string) (string, error) {
	if info, err := os.Stat(dir); err == nil {
		if info.IsDir() {
			return "", nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	first := dir
	for {
		parent := filepath.Dir(first)
		if parent == first {
			break
		}
		_, err := os.Stat(parent)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		first = parent
	}

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return "", err
	}
	return first, nil
}

// RemoveManagedLinks removes the given managed symlinks, logging progress to
// stdout. Entries that are no longer managed symlinks are left untouched. On
// failure it logs the stop position to stderr and returns the error.
func RemoveManagedLinks(targets []harness.Harness, ctx harness.Context, stdout, stderr io.Writer) error {
	items := make([]linkTarget, 0, len(targets))
	for _, h := range targets {
		items = append(items, linkTarget{Source: ctx.SourceFile, Target: h.Target})
	}
	return removeTargets(items, ctx, stdout, stderr)
}

// removeTargets removes every item, stopping at the first failure.
func removeTargets(items []linkTarget, ctx harness.Context, stdout, stderr io.Writer) error {
	for _, item := range items {
		if err := removeTarget(item, ctx, stdout); err != nil {
			fmt.Fprintf(stderr, "stopped at %s; earlier reported removals remain, and later destinations were not attempted.\n", item.Target)
			return err
		}
	}
	return nil
}

func removeTarget(item linkTarget, ctx harness.Context, stdout io.Writer) error {
	state, err := harness.TargetState(item.Target, item.Source)
	if err != nil {
		return err
	}
	switch state {
	case harness.Managed:
		if err := os.Remove(item.Target); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "removed managed symlink %s\n", harness.DisplayPath(item.Target, ctx.Home))
	case harness.Absent:
		fmt.Fprintf(stdout, "left untouched (already absent): %s\n", harness.DisplayPath(item.Target, ctx.Home))
	default:
		fmt.Fprintf(stdout, "left untouched (no longer a managed symlink): %s\n", harness.DisplayPath(item.Target, ctx.Home))
	}
	return nil
}

// ApplyReconcile applies a reconcile plan, creating links before removing any
// managed link so a failed addition cannot remove an existing link.
func ApplyReconcile(p ReconcilePlan, ctx harness.Context, stdout, stderr io.Writer) error {
	if err := CreateLinks(p.ToCreate, ctx, stdout, stderr); err != nil {
		if len(p.ToRemove) > 0 {
			fmt.Fprintln(stderr, "No planned managed links were removed because link creation did not finish.")
		}
		return err
	}
	return RemoveManagedLinks(p.ToRemove, ctx, stdout, stderr)
}

// LinkNotes returns the note text that the link, unlink, and reconcile flows
// print on stderr, or "" when there is none.
func LinkNotes(targets []harness.Harness, ctx harness.Context) (string, error) {
	var notes []string

	if hasHarness(targets, "codex") {
		override := filepath.Join(ctx.CodexHome, "AGENTS.override.md")
		info, err := os.Stat(override)
		if err == nil {
			if info.Size() > 0 {
				notes = append(notes, fmt.Sprintf("note: Codex prefers its non-empty %s over AGENTS.md", override))
			}
		} else if !harness.MissingPath(err) {
			return "", err
		}
	}

	if hasHarness(targets, "hermes") {
		notes = append(notes, "note: Hermes global AGENTS.md loading depends on Hermes version;\n      current docs guarantee global SOUL.md, not AGENTS.md")
	}

	return strings.Join(notes, "\n"), nil
}

func hasHarness(targets []harness.Harness, id string) bool {
	for _, h := range targets {
		if h.ID == id {
			return true
		}
	}
	return false
}
