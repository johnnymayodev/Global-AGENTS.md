package plan

import (
	"fmt"
	"strings"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// RenderLinkPlan renders the link plan the way the CLI prints it, without a
// trailing newline.
func RenderLinkPlan(p LinkPlan, ctx harness.Context) string {
	lines := []string{"Link plan:"}
	if len(p.ToCreate) > 0 {
		lines = append(lines, "  Create these symlinks (creating missing parent directories if needed):")
		lines = appendLinkPairs(lines, p.ToCreate, ctx)
	}
	if len(p.AlreadyLinked) > 0 {
		lines = append(lines, "  Already linked:")
		lines = appendLinkPairs(lines, p.AlreadyLinked, ctx)
	}
	if len(p.ToCreate) == 0 {
		lines = append(lines, "  No symlinks need to be created.")
	}
	lines = append(lines, "  Existing files and unrelated symlinks will not be overwritten.")
	return joinLines(lines)
}

// RenderUnlinkPlan renders the unlink plan the way the CLI prints it.
func RenderUnlinkPlan(p UnlinkPlan, ctx harness.Context) string {
	lines := []string{"Unlink plan:"}
	if len(p.ToRemove) > 0 {
		lines = append(lines, "  Remove only these managed symlink entries (not their target files):")
		lines = appendLinkPairs(lines, p.ToRemove, ctx)
	}
	for _, h := range p.AlreadyAbsent {
		lines = append(lines, "  Already absent: "+harness.DisplayPath(h.Target, ctx.Home))
	}
	for _, h := range p.Untouched {
		lines = append(lines, "  Leave untouched (not a managed symlink): "+harness.DisplayPath(h.Target, ctx.Home))
	}
	if len(p.ToRemove) == 0 {
		lines = append(lines, "  No managed symlinks will be removed.")
	}
	lines = append(lines, "  Regular files and unrelated symlinks will be left untouched.")
	return joinLines(lines)
}

// RenderReconcilePlan renders the combined plan of an interactive selection.
func RenderReconcilePlan(p ReconcilePlan, ctx harness.Context) string {
	lines := []string{"Apply plan:"}
	if len(p.ToCreate) > 0 {
		lines = append(lines, "  Create these symlinks (creating missing parent directories if needed):")
		lines = appendLinkPairs(lines, p.ToCreate, ctx)
	}
	if len(p.ToRemove) > 0 {
		lines = append(lines, "  Remove only these managed symlink entries (after confirmation):")
		lines = appendLinkPairs(lines, p.ToRemove, ctx)
	}
	if len(p.AlreadyLinked) > 0 {
		lines = append(lines, "  Keep already-linked destinations:")
		lines = appendLinkPairs(lines, p.AlreadyLinked, ctx)
	}
	for _, h := range p.AlreadyAbsent {
		lines = append(lines, "  Already absent: "+harness.DisplayPath(h.Target, ctx.Home))
	}
	for _, h := range p.Untouched {
		lines = append(lines, "  Leave untouched (not a managed symlink): "+harness.DisplayPath(h.Target, ctx.Home))
	}
	if len(p.ToCreate) == 0 && len(p.ToRemove) == 0 {
		lines = append(lines, "  No filesystem changes are planned.")
	}
	lines = append(lines, "  Existing files and unrelated symlinks will not be overwritten or removed.")
	return joinLines(lines)
}

func appendLinkPairs(lines []string, harnesses []harness.Harness, ctx harness.Context) []string {
	for _, h := range harnesses {
		lines = append(lines, fmt.Sprintf("    %s -> %s",
			harness.DisplayPath(h.Target, ctx.Home),
			harness.DisplayPath(ctx.SourceFile, ctx.Home),
		))
	}
	return lines
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
