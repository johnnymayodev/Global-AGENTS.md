// Package cli owns argument dispatch and the top-level error and exit-code
// wrapping.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/plan"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/ui"
)

// usage is the ported printUsage() text, printed for --help and unknown
// arguments.
const usage = `usage:
  agent-links                 choose detected harnesses interactively
  agent-links --all           choose from all supported harnesses
  agent-links --list [--all]  print the detected (or all) harnesses
  agent-links link            preview and confirm linking all supported harnesses
  agent-links unlink          preview and confirm removal of managed links`

// Options injects the process environment and streams. Run fills them from
// the real process; tests inject their own.
type Options struct {
	LookupEnv   harness.LookupEnv
	Cwd         string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool
}

// Run executes the CLI against the process environment, standard streams, and
// terminal status, returning the process exit code.
func Run(args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fail(os.Stderr, err)
	}
	return RunWith(args, Options{
		LookupEnv:   os.LookupEnv,
		Cwd:         cwd,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()),
	})
}

// RunWith executes the CLI with the given arguments and returns the process
// exit code.
func RunWith(args []string, opts Options) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(opts.Stdout, usage)
		return 0
	}

	ctx, err := harness.BuildContext(opts.LookupEnv, opts.Cwd)
	if err != nil {
		return fail(opts.Stderr, err)
	}

	prompt := ui.Prompter{
		In:          bufio.NewReader(opts.Stdin),
		Out:         opts.Stdout,
		Err:         opts.Stderr,
		Interactive: opts.Interactive,
	}

	var runErr error
	switch {
	case len(args) == 0 || (len(args) == 1 && args[0] == "--all"):
		runErr = pickerFlow(ctx, prompt, len(args) == 1 && args[0] == "--all")
	case args[0] == "--list" && (len(args) == 1 || (len(args) == 2 && args[1] == "--all")):
		runErr = listFlow(ctx, prompt, len(args) == 2)
	case len(args) == 1 && args[0] == "link":
		runErr = linkFlow(ctx, prompt)
	case len(args) == 1 && args[0] == "unlink":
		runErr = unlinkFlow(ctx, prompt)
	default:
		fmt.Fprintln(opts.Stdout, usage)
		return 2
	}
	if runErr != nil {
		return fail(opts.Stderr, runErr)
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 1
}

// pickerFlow mirrors the no-args and --all flows: ensure the source file,
// pick harnesses, then reconcile the selection.
func pickerFlow(ctx harness.Context, p ui.Prompter, showAll bool) error {
	if p.Interactive {
		return pickerFlowTUI(ctx, p, showAll)
	}
	return pickerFlowPlain(ctx, p, showAll)
}

// pickerFlowPlain prints the table and the non-interactive notice; choosing
// harnesses needs a terminal.
func pickerFlowPlain(ctx harness.Context, p ui.Prompter, showAll bool) error {
	ok, err := p.EnsureSourceFile(ctx)
	if err != nil || !ok {
		return err
	}

	visible, err := harness.Visible(ctx, showAll)
	if err != nil {
		return err
	}
	current, err := managedSelection(visible, ctx)
	if err != nil {
		return err
	}
	table, err := ui.RenderTable(visible, ctx, current, -1)
	if err != nil {
		return err
	}
	fmt.Fprintln(p.Out, table)
	fmt.Fprintln(p.Out, "Interactive selection needs a terminal; no changes made.")
	fmt.Fprintln(p.Out, "Cancelled. No changes made.")
	return nil
}

// pickerFlowTUI runs the full-screen wizard: the first-run setup when the
// source file is missing, the selection, and the plan review, as one
// alternate-screen program.
func pickerFlowTUI(ctx harness.Context, p ui.Prompter, showAll bool) error {
	exists, err := harness.SourceFileExists(ctx)
	if err != nil {
		return err
	}
	visible, err := harness.Visible(ctx, showAll)
	if err != nil {
		return err
	}
	current, err := managedSelection(visible, ctx)
	if err != nil {
		return err
	}

	if len(visible) == 0 {
		// Nothing to pick: still run first-run setup, then print the table.
		ok, err := p.EnsureSourceFile(ctx)
		if err != nil || !ok {
			return err
		}
		table, err := ui.RenderTable(visible, ctx, current, -1)
		if err != nil {
			return err
		}
		fmt.Fprintln(p.Out, table)
		fmt.Fprintln(p.Out, "No supported harnesses were detected. Use `agent-links --all` to show every supported target.")
		fmt.Fprintln(p.Out, "Cancelled. No changes made.")
		return nil
	}

	result, err := ui.RunWizard(ui.WizardFlow{
		Harnesses: visible,
		Ctx:       ctx,
		Initial:   current,
		Setup:     !exists,
		Review:    reconcileReview(visible, ctx, p),
	})
	if err != nil {
		return err
	}
	for _, notice := range result.Notices {
		fmt.Fprintln(p.Out, notice)
	}
	switch {
	case result.Cancelled:
		fmt.Fprintln(p.Out, "Cancelled. No changes made.")
	case result.NoChanges:
		fmt.Fprintln(p.Out, "No changes made.")
	case result.Applied:
		fmt.Fprintln(p.Out, result.Review.Summary)
		return result.Review.Apply()
	}
	return nil
}

// listFlow prints the detected (or all) harnesses as a table.
func listFlow(ctx harness.Context, p ui.Prompter, showAll bool) error {
	harnesses, err := harness.Visible(ctx, showAll)
	if err != nil {
		return err
	}
	selected, err := managedSelection(harnesses, ctx)
	if err != nil {
		return err
	}
	table, err := ui.RenderTable(harnesses, ctx, selected, -1)
	if err != nil {
		return err
	}
	fmt.Fprintln(p.Out, table)
	if len(harnesses) == 0 {
		fmt.Fprintln(p.Out, "No supported harnesses were detected. Use `agent-links --all` to show every supported target.")
	}
	return nil
}

// linkFlow ensures the source file and links every supported harness.
func linkFlow(ctx harness.Context, p ui.Prompter) error {
	ok, err := p.EnsureSourceFile(ctx)
	if err != nil || !ok {
		return err
	}
	return linkTargets(ctx.Harnesses, ctx, p)
}

func linkTargets(targets []harness.Harness, ctx harness.Context, p ui.Prompter) error {
	linkPlan, err := plan.Links(targets, ctx)
	if err != nil {
		return err
	}
	planText := plan.RenderLinkPlan(linkPlan, ctx)
	if len(linkPlan.ToCreate) == 0 {
		fmt.Fprintln(p.Out, planText)
		fmt.Fprintln(p.Out, "No changes made.")
		return printLinkNotes(targets, ctx, p.Err)
	}

	confirmed, err := p.Confirm(
		planText,
		fmt.Sprintf(`Type "yes" to create the %d symlink(s) listed above: `, len(linkPlan.ToCreate)),
		fmt.Sprintf("Confirmation received; creating %d symlink(s).", len(linkPlan.ToCreate)),
		"",
	)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	if err := plan.CreateLinks(linkPlan.ToCreate, ctx, p.Out, p.Err); err != nil {
		return err
	}
	return printLinkNotes(targets, ctx, p.Err)
}

// unlinkFlow removes managed links; removal needs interactive confirmation.
func unlinkFlow(ctx harness.Context, p ui.Prompter) error {
	unlinkPlan, err := plan.Unlinks(ctx.Harnesses, ctx)
	if err != nil {
		return err
	}
	planText := plan.RenderUnlinkPlan(unlinkPlan, ctx)
	if len(unlinkPlan.ToRemove) == 0 {
		fmt.Fprintln(p.Out, planText)
		fmt.Fprintln(p.Out, "No changes made.")
		return nil
	}

	confirmed, err := p.Confirm(
		planText,
		fmt.Sprintf(`Type "yes" to remove the %d managed symlink(s) listed above: `, len(unlinkPlan.ToRemove)),
		fmt.Sprintf("Confirmation received; removing %d managed symlink(s).", len(unlinkPlan.ToRemove)),
		"removal requires interactive confirmation; no changes made",
	)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	return plan.RemoveManagedLinks(unlinkPlan.ToRemove, ctx, p.Out, p.Err)
}

// reconcileReview adapts the picker's reconciliation to the wizard's review
// step: it builds the plan text, the confirmation line, and the apply closure
// from one reconcile pass, so the plan the user approved is exactly the plan
// that runs.
func reconcileReview(visible []harness.Harness, ctx harness.Context, p ui.Prompter) func(map[string]bool) (ui.Review, bool, error) {
	return func(selected map[string]bool) (ui.Review, bool, error) {
		reconcilePlan, err := plan.Reconcile(visible, selected, ctx)
		if err != nil {
			return ui.Review{}, false, err
		}
		if len(reconcilePlan.ToCreate) == 0 && len(reconcilePlan.ToRemove) == 0 {
			return ui.Review{}, false, nil
		}

		var changes []string
		if len(reconcilePlan.ToCreate) > 0 {
			changes = append(changes, fmt.Sprintf("create %d symlink(s)", len(reconcilePlan.ToCreate)))
		}
		if len(reconcilePlan.ToRemove) > 0 {
			changes = append(changes, fmt.Sprintf("remove %d managed symlink(s)", len(reconcilePlan.ToRemove)))
		}
		summary := strings.Join(changes, " and ")

		return ui.Review{
			Text:    plan.RenderReconcilePlan(reconcilePlan, ctx),
			Summary: fmt.Sprintf("Confirmation received; applying plan (%s).", summary),
			Apply: func() error {
				if err := plan.ApplyReconcile(reconcilePlan, ctx, p.Out, p.Err); err != nil {
					return err
				}
				var chosen []harness.Harness
				for _, h := range visible {
					if selected[h.ID] {
						chosen = append(chosen, h)
					}
				}
				return printLinkNotes(chosen, ctx, p.Err)
			},
		}, true, nil
	}
}

// managedSelection collects the IDs whose target is already a managed link.
func managedSelection(harnesses []harness.Harness, ctx harness.Context) (map[string]bool, error) {
	selected := make(map[string]bool)
	for _, h := range harnesses {
		state, err := harness.TargetState(h.Target, ctx.SourceFile)
		if err != nil {
			return nil, err
		}
		if state == harness.Managed {
			selected[h.ID] = true
		}
	}
	return selected, nil
}

func printLinkNotes(targets []harness.Harness, ctx harness.Context, stderr io.Writer) error {
	notes, err := plan.LinkNotes(targets, ctx)
	if err != nil {
		return err
	}
	if notes != "" {
		fmt.Fprintln(stderr, notes)
	}
	return nil
}
