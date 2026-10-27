package plan

import (
	"fmt"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// RenderSkillLinkPlan renders a one-skill link plan, without a trailing
// newline.
func RenderSkillLinkPlan(p SkillsPlan, ctx harness.Context) string {
	lines := []string{"Link plan:"}
	if len(p.ToCreate) > 0 {
		lines = append(lines, "  Create these symlinks (creating missing parent directories if needed):")
		lines = appendSkillPairs(lines, p.ToCreate, ctx)
	}
	if len(p.AlreadyLinked) > 0 {
		lines = append(lines, "  Already linked:")
		lines = appendSkillPairs(lines, p.AlreadyLinked, ctx)
	}
	if len(p.Unavailable) > 0 {
		lines = append(lines, "  Unavailable skill/harness pairs:")
		lines = appendUnavailable(lines, p.Unavailable, ctx)
	}
	if len(p.ToCreate) == 0 {
		lines = append(lines, "  No symlinks need to be created.")
	}
	lines = append(lines, "  Existing files and unrelated symlinks will not be overwritten.")
	return joinLines(lines)
}

// RenderSkillUnlinkPlan renders a one-skill unlink plan, without a trailing
// newline.
func RenderSkillUnlinkPlan(p SkillsPlan, ctx harness.Context) string {
	lines := []string{"Unlink plan:"}
	if len(p.ToRemove) > 0 {
		lines = append(lines, "  Remove only these managed symlink entries (not their target files):")
		lines = appendSkillPairs(lines, p.ToRemove, ctx)
	}
	for _, cell := range p.AlreadyAbsent {
		lines = append(lines, "  Already absent: "+harness.DisplayPath(cell.Target, ctx.Home))
	}
	for _, cell := range p.Unmanaged {
		lines = append(lines, "  Leave untouched (not a managed symlink): "+harness.DisplayPath(cell.Target, ctx.Home))
	}
	if len(p.ToRemove) == 0 {
		lines = append(lines, "  No managed symlinks will be removed.")
	}
	lines = append(lines, "  Regular files and unrelated symlinks will be left untouched.")
	return joinLines(lines)
}

// RenderSkillsReconcilePlan renders the combined plan of a skills matrix
// selection. Unchanged cells are summarized rather than listed.
func RenderSkillsReconcilePlan(p SkillsPlan, ctx harness.Context) string {
	lines := []string{"Apply plan:"}
	if len(p.ToCreate) > 0 {
		lines = append(lines, "  Create these symlinks (creating missing parent directories if needed):")
		lines = appendSkillPairs(lines, p.ToCreate, ctx)
	}
	if len(p.ToRemove) > 0 {
		lines = append(lines, "  Remove only these managed symlink entries (after confirmation):")
		lines = appendSkillPairs(lines, p.ToRemove, ctx)
	}
	if len(p.AlreadyLinked) > 0 || len(p.AlreadyAbsent) > 0 {
		lines = append(lines, fmt.Sprintf("  Keep %d already-linked and %d absent destination(s) unchanged.",
			len(p.AlreadyLinked), len(p.AlreadyAbsent)))
	}
	if len(p.Unmanaged) > 0 {
		lines = append(lines, fmt.Sprintf("  Leave %d conflict cell(s) untouched.", len(p.Unmanaged)))
	}
	if len(p.Unavailable) > 0 {
		lines = append(lines, fmt.Sprintf("  Leave %d statically unavailable cell(s) untouched.", len(p.Unavailable)))
	}
	if len(p.ToCreate) == 0 && len(p.ToRemove) == 0 {
		lines = append(lines, "  No filesystem changes are planned.")
	}
	lines = append(lines, "  Existing files and unrelated symlinks will not be overwritten or removed.")
	return joinLines(lines)
}

func appendSkillPairs(lines []string, cells []SkillCell, ctx harness.Context) []string {
	for _, cell := range cells {
		lines = append(lines, fmt.Sprintf("    %s -> %s",
			harness.DisplayPath(cell.Target, ctx.Home),
			harness.DisplayPath(cell.Source, ctx.Home),
		))
	}
	return lines
}

func appendUnavailable(lines []string, cells []SkillCell, ctx harness.Context) []string {
	for _, cell := range cells {
		lines = append(lines, fmt.Sprintf("    %s: %s",
			harness.DisplayPath(cell.Target, ctx.Home),
			cell.Unavailable,
		))
	}
	return lines
}
