// Package cli owns argument dispatch and the top-level error and exit-code
// wrapping.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/plan"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/skill"
	"github.com/johnnymayodev/Global-AGENTS.md/internal/ui"
)

// usage is printed for --help and unknown arguments.
const usage = `usage:
  agent-links                          choose instructions or skills, then harnesses
  agent-links --all                    choose instructions or skills, then all harnesses
  agent-links --list [--all]           print the detected (or all) instruction targets
  agent-links link                     preview and confirm linking all supported harnesses
  agent-links unlink                   preview and confirm removal of managed links
  agent-links skills [--all]           manage shared skills for detected (or all) harnesses
  agent-links skills --list [--all]    print the shared skills matrix
  agent-links skills link <skill>      preview and confirm linking one skill
  agent-links skills unlink <skill>    preview and confirm unlinking one skill
  agent-links --version                print the version`

// Options injects the process environment and streams. Run fills them from
// the real process; tests inject their own.
type Options struct {
	LookupEnv   harness.LookupEnv
	Cwd         string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool

	// TTY is the controlling terminal handed to Bubble Tea as its input.
	// Run opens and closes it when the session is interactive; RunWith leaves
	// it nil in tests, which keeps the inherited-stdin behavior. RunWith does
	// not close a caller-supplied TTY.
	TTY *os.File
}

// Run executes the CLI against the process environment, standard streams, and
// terminal status, returning the process exit code.
func Run(args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fail(os.Stderr, err)
	}
	interactive := term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
	opts := Options{
		LookupEnv:   os.LookupEnv,
		Cwd:         cwd,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: interactive,
	}
	if interactive {
		// Bubble Tea's cancel reader picks its macOS kqueue path when the
		// input file's name is not /dev/tty, and that path cannot interrupt a
		// read on a terminal opened by the shell as </dev/tty. Opening the
		// controlling terminal here makes it a select-based reader and removes
		// the 500 ms shutdown stall. If this fails, stdin remains a usable
		// fallback.
		if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
			if term.IsTerminal(tty.Fd()) {
				opts.TTY = tty
				defer tty.Close()
			} else {
				tty.Close()
			}
		}
	}
	return RunWith(args, opts)
}

// RunWith executes the CLI with the given arguments and returns the process
// exit code.
func RunWith(args []string, opts Options) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(opts.Stdout, usage)
		return 0
	}
	// Version reporting needs neither HOME nor a working directory.
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
		printVersion(opts.Stdout)
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
		TTY:         opts.TTY,
	}

	var runErr error
	switch {
	case len(args) == 0 || (len(args) == 1 && args[0] == "--all"):
		runErr = launchFlow(ctx, prompt, len(args) == 1 && args[0] == "--all")
	case args[0] == "--list" && (len(args) == 1 || (len(args) == 2 && args[1] == "--all")):
		runErr = listFlow(ctx, prompt, len(args) == 2)
	case len(args) == 1 && args[0] == "link":
		runErr = linkFlow(ctx, prompt)
	case len(args) == 1 && args[0] == "unlink":
		runErr = unlinkFlow(ctx, prompt)
	case args[0] == "skills":
		inv, ok := parseSkillsInvocation(args[1:])
		if !ok {
			fmt.Fprintln(opts.Stdout, usage)
			return 2
		}
		if inv.mode == skillsModeHelp {
			fmt.Fprintln(opts.Stdout, usage)
			return 0
		}
		runErr = skillsFlowDispatch(ctx, prompt, inv)
	default:
		fmt.Fprintln(opts.Stdout, usage)
		return 2
	}
	if runErr != nil {
		return fail(opts.Stderr, runErr)
	}
	return 0
}

// skillsMode identifies the flow one parsed "skills" invocation selects.
type skillsMode int

const (
	skillsModeManage skillsMode = iota
	skillsModeList
	skillsModeLink
	skillsModeUnlink
	skillsModeHelp
)

// skillsInvocation is one parsed "skills" command line.
type skillsInvocation struct {
	mode    skillsMode
	showAll bool
	name    string
}

// parseSkillsInvocation parses the arguments after "skills" once, so the
// dispatch never reinterprets them. It owns every flag: -h/--help in the
// command position selects help, --all widens a manage or list invocation, and
// a flag where a skill name belongs is invalid usage. Only "--" before the name
// lets the name begin with "-". skill.ValidateName remains authoritative for
// the name itself.
func parseSkillsInvocation(args []string) (skillsInvocation, bool) {
	if len(args) == 0 {
		return skillsInvocation{mode: skillsModeManage}, true
	}
	switch args[0] {
	case "-h", "--help":
		if len(args) == 1 {
			return skillsInvocation{mode: skillsModeHelp}, true
		}
	case "--all":
		if len(args) == 1 {
			return skillsInvocation{mode: skillsModeManage, showAll: true}, true
		}
	case "--list":
		inv := skillsInvocation{mode: skillsModeList}
		rest := args[1:]
		if len(rest) > 0 && rest[0] == "--all" {
			inv.showAll = true
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return inv, true
		}
	case "link", "unlink":
		inv := skillsInvocation{mode: skillsModeLink}
		if args[0] == "unlink" {
			inv.mode = skillsModeUnlink
		}
		rest := args[1:]
		terminated := false
		if len(rest) > 0 && rest[0] == "--" {
			terminated = true
			rest = rest[1:]
		}
		if len(rest) != 1 {
			return skillsInvocation{}, false
		}
		if !terminated && strings.HasPrefix(rest[0], "-") {
			return skillsInvocation{}, false
		}
		inv.name = rest[0]
		return inv, true
	}
	return skillsInvocation{}, false
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 1
}

// launchFlow offers the instruction and skills modes, then runs the chosen
// flow.
func launchFlow(ctx harness.Context, p ui.Prompter, showAll bool) error {
	if p.Interactive {
		return launchFlowTUI(ctx, p, showAll)
	}
	return launchFlowPlain(ctx, p, showAll)
}

func launchFlowPlain(ctx harness.Context, p ui.Prompter, showAll bool) error {
	fmt.Fprintln(p.Out, "What do you want to manage?")
	for {
		choice, ok := p.Ask("Choose: [1] global instructions (AGENTS.md)  [2] global skills (SKILL.md)  [q] cancel: ")
		if !ok {
			fmt.Fprintln(p.Out, "Cancelled. No changes made.")
			return nil
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "q":
			fmt.Fprintln(p.Out, "Cancelled. No changes made.")
			return nil
		case "1":
			return pickerFlowPlain(ctx, p, showAll)
		case "2":
			return skillsFlowPlain(ctx, p, showAll)
		default:
			fmt.Fprintln(p.Out, `Enter "1" to manage global instructions, "2" to manage global skills, or "q" to cancel.`)
		}
	}
}

func launchFlowTUI(ctx harness.Context, p ui.Prompter, showAll bool) error {
	var instructionsErr error
	exists, err := harness.SourceFileExists(ctx)
	if err != nil {
		instructionsErr = err
	}
	var visibleInstructions []harness.Harness
	var current map[string]bool
	if instructionsErr == nil {
		visibleInstructions, err = harness.Visible(ctx, showAll)
		if err != nil {
			instructionsErr = err
		} else {
			current, err = managedSelection(visibleInstructions, ctx)
			if err != nil {
				instructionsErr = err
			}
		}
	}

	var skillsErr error
	var skills []skill.Skill
	var matrix ui.SkillsMatrix
	skills, err = skill.Discover(ctx.SourceSkillsDir)
	if err != nil {
		skillsErr = err
	} else {
		visibleSkills, err := harness.VisibleSkills(ctx, showAll)
		if err != nil {
			skillsErr = err
		} else {
			matrix, err = buildSkillsMatrix(ctx, skills, visibleSkills)
			if err != nil {
				skillsErr = err
			}
		}
	}

	result, err := ui.RunWizard(ui.WizardFlow{
		Harnesses:         visibleInstructions,
		Ctx:               ctx,
		Initial:           current,
		Setup:             instructionsErr == nil && !exists,
		Review:            reconcileReview(visibleInstructions, ctx, p),
		Launch:            true,
		InstructionsError: instructionsErr,
		SkillsError:       skillsErr,
		Skills:            &matrix,
		SkillsReview:      skillsReconcileReview(matrix, ctx, p),
	}, p.TTY)
	if err != nil {
		return err
	}
	for _, notice := range result.Notices {
		fmt.Fprintln(p.Out, notice)
	}
	if line := wizardOutcome(result); line != "" {
		fmt.Fprintln(p.Out, line)
	}
	if result.Applied {
		fmt.Fprintln(p.Out, result.Review.Summary)
		return result.Review.Apply()
	}
	return nil
}

// wizardOutcome is the wizard's closing line. When setup created the
// instructions file, the only thing that could remain unchanged is the links.
func wizardOutcome(result ui.WizardResult) string {
	switch {
	case result.Cancelled:
		if result.Created {
			return "Cancelled. No links were changed."
		}
		return "Cancelled. No changes made."
	case result.NoChanges:
		if result.Created {
			return "No links were changed."
		}
		return "No changes made."
	}
	return ""
}

// pickerFlowPlain prints the table and the non-interactive notice; choosing
// harnesses needs a terminal.
func pickerFlowPlain(ctx harness.Context, p ui.Prompter, showAll bool) error {
	ok, _, err := p.EnsureSourceFile(ctx)
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
	ok, created, err := p.EnsureSourceFile(ctx)
	if err != nil || !ok {
		return err
	}
	return linkTargets(harness.WithInstructions(ctx.Harnesses), ctx, p, created)
}

// linkNoChangesLine is the no-op outcome for a link plan. Interactive runs
// that created the instructions file this run report that only the links were
// unchanged; plain-text output keeps its historical wording.
func linkNoChangesLine(created, interactive bool) string {
	if created && interactive {
		return "No links were changed."
	}
	return "No changes made."
}

func linkTargets(targets []harness.Harness, ctx harness.Context, p ui.Prompter, created bool) error {
	linkPlan, err := plan.Links(targets, ctx)
	if err != nil {
		return err
	}
	notes, err := plan.LinkNotes(targets, ctx)
	if err != nil {
		return err
	}
	planText := plan.RenderLinkPlan(linkPlan, ctx)
	if len(linkPlan.ToCreate) == 0 {
		fmt.Fprintln(p.Out, planText)
		fmt.Fprintln(p.Out, linkNoChangesLine(created, p.Interactive))
		printLinkNotes(notes, p.Err)
		return nil
	}

	confirmed, err := p.Confirm(
		planText,
		fmt.Sprintf(`Type "yes" to create the %d symlink(s) listed above: `, len(linkPlan.ToCreate)),
		fmt.Sprintf("Confirmation received; creating %d symlink(s).", len(linkPlan.ToCreate)),
		"",
		created,
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
	printLinkNotes(notes, p.Err)
	return nil
}

// unlinkFlow removes managed links; removal needs interactive confirmation.
func unlinkFlow(ctx harness.Context, p ui.Prompter) error {
	unlinkPlan, err := plan.Unlinks(harness.WithInstructions(ctx.Harnesses), ctx)
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
		false,
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

		var chosen []harness.Harness
		for _, h := range visible {
			if selected[h.ID] {
				chosen = append(chosen, h)
			}
		}
		notes, err := plan.LinkNotes(chosen, ctx)
		if err != nil {
			return ui.Review{}, false, err
		}

		return ui.Review{
			Text:    plan.RenderReconcilePlan(reconcilePlan, ctx),
			Summary: fmt.Sprintf("Confirmation received; applying plan (%s).", summary),
			Apply: func() error {
				if err := plan.ApplyReconcile(reconcilePlan, ctx, p.Out, p.Err); err != nil {
					return err
				}
				printLinkNotes(notes, p.Err)
				return nil
			},
		}, true, nil
	}
}

// skillsReconcileReview builds the review for a matrix selection.
func skillsReconcileReview(matrix ui.SkillsMatrix, ctx harness.Context, p ui.Prompter) func(map[string]bool) (ui.Review, bool, error) {
	return func(selected map[string]bool) (ui.Review, bool, error) {
		cells := matrixPlanCells(matrix)
		skillsPlan, err := plan.SkillReconcile(cells, selected)
		if err != nil {
			return ui.Review{}, false, err
		}
		if len(skillsPlan.ToCreate) == 0 && len(skillsPlan.ToRemove) == 0 {
			return ui.Review{}, false, nil
		}

		var changes []string
		if len(skillsPlan.ToCreate) > 0 {
			changes = append(changes, fmt.Sprintf("create %d symlink(s)", len(skillsPlan.ToCreate)))
		}
		if len(skillsPlan.ToRemove) > 0 {
			changes = append(changes, fmt.Sprintf("remove %d managed symlink(s)", len(skillsPlan.ToRemove)))
		}
		summary := strings.Join(changes, " and ")

		return ui.Review{
			Text:    plan.RenderSkillsReconcilePlan(skillsPlan, ctx),
			Summary: fmt.Sprintf("Confirmation received; applying plan (%s).", summary),
			Apply: func() error {
				return plan.ApplySkills(skillsPlan, ctx, p.Out, p.Err)
			},
		}, true, nil
	}
}

// skillsFlowDispatch runs the parsed skills invocation.
func skillsFlowDispatch(ctx harness.Context, p ui.Prompter, inv skillsInvocation) error {
	switch inv.mode {
	case skillsModeManage:
		return skillsFlow(ctx, p, inv.showAll)
	case skillsModeList:
		return skillsListFlow(ctx, p, inv.showAll)
	case skillsModeLink:
		return skillsLinkFlow(ctx, p, inv.name)
	case skillsModeUnlink:
		return skillsUnlinkFlow(ctx, p, inv.name)
	}
	return nil
}

// skillsFlow runs the interactive skills matrix.
func skillsFlow(ctx harness.Context, p ui.Prompter, showAll bool) error {
	skills, err := skill.Discover(ctx.SourceSkillsDir)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		printNoSkills(ctx, p.Out)
		return nil
	}

	visible, err := harness.VisibleSkills(ctx, showAll)
	if err != nil {
		return err
	}
	matrix, err := buildSkillsMatrix(ctx, skills, visible)
	if err != nil {
		return err
	}

	if !p.Interactive {
		table, err := ui.RenderSkillsTable(matrix, ctx, managedSkillSelection(matrix))
		if err != nil {
			return err
		}
		fmt.Fprintln(p.Out, table)
		if len(visible) == 0 {
			fmt.Fprintln(p.Out, "No supported harnesses were detected. Use `agent-links skills --all` to show every supported target.")
		}
		fmt.Fprintln(p.Out, "Interactive selection needs a terminal; no changes made.")
		fmt.Fprintln(p.Out, "Cancelled. No changes made.")
		return nil
	}

	if len(visible) == 0 {
		table, err := ui.RenderSkillsTable(matrix, ctx, managedSkillSelection(matrix))
		if err != nil {
			return err
		}
		fmt.Fprintln(p.Out, table)
		fmt.Fprintln(p.Out, "No supported harnesses were detected. Use `agent-links skills --all` to show every supported target.")
		fmt.Fprintln(p.Out, "No changes made.")
		return nil
	}

	result, err := ui.RunWizard(ui.WizardFlow{
		Ctx:          ctx,
		Mode:         ui.ModeSkills,
		Skills:       &matrix,
		SkillsReview: skillsReconcileReview(matrix, ctx, p),
	}, p.TTY)
	if err != nil {
		return err
	}
	for _, notice := range result.Notices {
		fmt.Fprintln(p.Out, notice)
	}
	if line := wizardOutcome(result); line != "" {
		fmt.Fprintln(p.Out, line)
	}
	if result.Applied {
		fmt.Fprintln(p.Out, result.Review.Summary)
		return result.Review.Apply()
	}
	return nil
}

// skillsFlowPlain prints the read-only skills matrix when no terminal is
// available.
func skillsFlowPlain(ctx harness.Context, p ui.Prompter, showAll bool) error {
	skills, err := skill.Discover(ctx.SourceSkillsDir)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		printNoSkills(ctx, p.Out)
		return nil
	}
	visible, err := harness.VisibleSkills(ctx, showAll)
	if err != nil {
		return err
	}
	matrix, err := buildSkillsMatrix(ctx, skills, visible)
	if err != nil {
		return err
	}
	table, err := ui.RenderSkillsTable(matrix, ctx, managedSkillSelection(matrix))
	if err != nil {
		return err
	}
	fmt.Fprintln(p.Out, table)
	if len(visible) == 0 {
		fmt.Fprintln(p.Out, "No supported harnesses were detected. Use `agent-links skills --all` to show every supported target.")
	}
	fmt.Fprintln(p.Out, "Interactive selection needs a terminal; no changes made.")
	fmt.Fprintln(p.Out, "Cancelled. No changes made.")
	return nil
}

// skillsListFlow prints the read-only skills matrix.
func skillsListFlow(ctx harness.Context, p ui.Prompter, showAll bool) error {
	skills, err := skill.Discover(ctx.SourceSkillsDir)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		printNoSkills(ctx, p.Out)
		return nil
	}
	visible, err := harness.VisibleSkills(ctx, showAll)
	if err != nil {
		return err
	}
	matrix, err := buildSkillsMatrix(ctx, skills, visible)
	if err != nil {
		return err
	}
	table, err := ui.RenderSkillsTable(matrix, ctx, managedSkillSelection(matrix))
	if err != nil {
		return err
	}
	fmt.Fprintln(p.Out, table)
	if len(visible) == 0 {
		fmt.Fprintln(p.Out, "No supported harnesses were detected. Use `agent-links skills --all` to show every supported target.")
	}
	return nil
}

// skillsLinkFlow links one named skill into every supported harness.
func skillsLinkFlow(ctx harness.Context, p ui.Prompter, name string) error {
	s, err := skill.Find(ctx.SourceSkillsDir, name)
	if err != nil {
		return err
	}
	cells := skillCells([]skill.Skill{s}, ctx.Harnesses, true)
	skillsPlan, err := plan.SkillLinks(cells)
	if err != nil {
		return err
	}
	planText := plan.RenderSkillLinkPlan(skillsPlan, ctx)
	if len(skillsPlan.ToCreate) == 0 {
		fmt.Fprintln(p.Out, planText)
		fmt.Fprintln(p.Out, "No changes made.")
		return nil
	}

	confirmed, err := p.Confirm(
		planText,
		fmt.Sprintf(`Type "yes" to create the %d symlink(s) listed above: `, len(skillsPlan.ToCreate)),
		fmt.Sprintf("Confirmation received; creating %d symlink(s).", len(skillsPlan.ToCreate)),
		"",
		false,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	return plan.CreateSkillLinks(skillsPlan.ToCreate, ctx, p.Out, p.Err)
}

// skillsUnlinkFlow removes one named skill's managed links. The source need
// not exist, and reserved names can still be cleaned up.
func skillsUnlinkFlow(ctx harness.Context, p ui.Prompter, name string) error {
	if err := skill.ValidateName(name); err != nil {
		return err
	}
	s := skill.Skill{Name: name, SourceDir: filepath.Join(ctx.SourceSkillsDir, name)}
	cells := skillCells([]skill.Skill{s}, ctx.Harnesses, false)
	skillsPlan, err := plan.SkillUnlinks(cells)
	if err != nil {
		return err
	}
	planText := plan.RenderSkillUnlinkPlan(skillsPlan, ctx)
	if len(skillsPlan.ToRemove) == 0 {
		fmt.Fprintln(p.Out, planText)
		fmt.Fprintln(p.Out, "No changes made.")
		return nil
	}

	confirmed, err := p.Confirm(
		planText,
		fmt.Sprintf(`Type "yes" to remove the %d managed symlink(s) listed above: `, len(skillsPlan.ToRemove)),
		fmt.Sprintf("Confirmation received; removing %d managed symlink(s).", len(skillsPlan.ToRemove)),
		"removal requires interactive confirmation; no changes made",
		false,
	)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}
	return plan.RemoveManagedSkillLinks(skillsPlan.ToRemove, ctx, p.Out, p.Err)
}

// buildSkillsMatrix classifies every skill/harness cell and selects the
// destinations that are already managed.
func buildSkillsMatrix(ctx harness.Context, skills []skill.Skill, harnesses []harness.Harness) (ui.SkillsMatrix, error) {
	matrix := ui.SkillsMatrix{
		Skills:    make([]string, 0, len(skills)),
		Harnesses: harnesses,
		Cells:     make(map[string]ui.MatrixCell),
		Initial:   make(map[string]bool),
	}
	for _, s := range skills {
		matrix.Skills = append(matrix.Skills, s.Name)
		for _, h := range harnesses {
			target := filepath.Join(h.SkillsDir, s.Name)
			state, err := harness.TargetState(target, s.SourceDir)
			if err != nil {
				return ui.SkillsMatrix{}, err
			}
			cell := ui.MatrixCell{
				Skill:       s.Name,
				HarnessID:   h.ID,
				HarnessName: h.Name,
				Source:      s.SourceDir,
				Target:      target,
				State:       state,
				Unavailable: harness.SkillUnavailableReason(h, s.Name),
			}
			matrix.Cells[cell.Key()] = cell
			if state == harness.Managed {
				matrix.Initial[cell.Key()] = true
			}
		}
	}
	return matrix, nil
}

// managedSkillSelection collects cells whose destination is already a managed
// link.
func managedSkillSelection(matrix ui.SkillsMatrix) map[string]bool {
	selected := make(map[string]bool)
	for key, cell := range matrix.Cells {
		if cell.State == harness.Managed {
			selected[key] = true
		}
	}
	return selected
}

// matrixPlanCells converts a matrix into plan cells in stable row-major order.
func matrixPlanCells(matrix ui.SkillsMatrix) []plan.SkillCell {
	cells := make([]plan.SkillCell, 0, len(matrix.Cells))
	for _, skillName := range matrix.Skills {
		for _, h := range matrix.Harnesses {
			cell, ok := matrix.Cell(skillName, h.ID)
			if !ok {
				continue
			}
			cells = append(cells, plan.SkillCell{
				Skill:       cell.Skill,
				HarnessID:   cell.HarnessID,
				HarnessName: cell.HarnessName,
				Source:      cell.Source,
				Target:      cell.Target,
				Unavailable: harness.SkillUnavailableReason(h, skillName),
			})
		}
	}
	return cells
}

// skillCells builds plan cells for one or more skills across harnesses.
func skillCells(skills []skill.Skill, harnesses []harness.Harness, withAvailability bool) []plan.SkillCell {
	cells := make([]plan.SkillCell, 0, len(skills)*len(harnesses))
	for _, s := range skills {
		for _, h := range harnesses {
			cell := plan.SkillCell{
				Skill:       s.Name,
				HarnessID:   h.ID,
				HarnessName: h.Name,
				Source:      s.SourceDir,
				Target:      filepath.Join(h.SkillsDir, s.Name),
			}
			if withAvailability {
				cell.Unavailable = harness.SkillUnavailableReason(h, s.Name)
			}
			cells = append(cells, cell)
		}
	}
	return cells
}

func printNoSkills(ctx harness.Context, out io.Writer) {
	fmt.Fprintf(out, "No shared skills found at %s.\n", harness.DisplayPath(ctx.SourceSkillsDir, ctx.Home))
	fmt.Fprintln(out, "Add <skill-name>/SKILL.md (plus any supporting files), then rerun agent-links.")
	fmt.Fprintln(out, "No changes made.")
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

func printLinkNotes(notes string, stderr io.Writer) {
	if notes != "" {
		fmt.Fprintln(stderr, notes)
	}
}
