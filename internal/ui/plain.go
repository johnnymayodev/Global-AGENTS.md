// Package ui renders the interactive and plain-text user interface: the
// launch mode menu, the canonical-file setup flow, the instruction and skills
// pickers, and the confirmation prompts.
package ui

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// Prompter runs the interactions: the Bubble Tea models in tui.go when stdin
// and stdout are both terminals, and otherwise the plain-text path: line
// prompts over stdin and stdout, errors over stderr.
type Prompter struct {
	In          *bufio.Reader
	Out         io.Writer
	Err         io.Writer
	Interactive bool

	// TTY, when non-nil, is the controlling terminal handed to Bubble Tea as
	// its input. The CLI owns and closes it; the Prompter only reads from it.
	TTY io.Reader
}

// Ask writes the prompt and reads one line. ok is false when stdin is closed
// before any input; a final line without a trailing newline still counts, as
// in the TypeScript readline implementation.
func (p Prompter) Ask(message string) (answer string, ok bool) {
	fmt.Fprint(p.Out, message)
	line, err := p.In.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimRight(line, "\r\n"), true
}

// EnsureSourceFile reports whether the canonical instructions file exists,
// running the copy / blank / cancel setup flow when it is missing. created
// reports that this run created the file, so the interactive outcomes can say
// that links, not files, were the only thing left unchanged.
func (p Prompter) EnsureSourceFile(ctx harness.Context) (ok bool, created bool, err error) {
	exists, err := harness.SourceFileExists(ctx)
	if err != nil {
		return false, false, err
	}
	if exists {
		return true, false, nil
	}
	if p.Interactive {
		return p.ensureSourceFileTUI(ctx)
	}

	fmt.Fprintf(p.Out, "No shared instructions file found at %s.\n", ctx.SourceFile)
	for {
		choice, ok := p.Ask("Choose: [1] copy an existing instructions file  [2] create a blank AGENTS.md  [q] cancel: ")
		if !ok || strings.ToLower(strings.TrimSpace(choice)) == "q" {
			fmt.Fprintln(p.Out, "Cancelled. No changes made.")
			return false, false, nil
		}

		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1":
			path, ok := p.Ask("Path to the existing instructions file: ")
			if !ok {
				fmt.Fprintln(p.Out, "Cancelled. No changes made.")
				return false, false, nil
			}
			selectedPath := SelectedFilePath(path, ctx.Home, ctx.Cwd())
			if err := CheckSelectedFile(selectedPath); err != nil {
				fmt.Fprintln(p.Err, err)
				continue
			}
			if err := CopySourceFile(ctx.SourceFile, selectedPath); err != nil {
				return false, false, err
			}
			fmt.Fprintf(p.Out, "Created %s from %s\n", ctx.SourceFile, selectedPath)
			return true, true, nil
		case "2":
			if err := CreateBlankSource(ctx.SourceFile); err != nil {
				return false, false, err
			}
			fmt.Fprintf(p.Out, "Created blank instructions file: %s\n", ctx.SourceFile)
			return true, true, nil
		default:
			fmt.Fprintln(p.Out, `Enter "1" to copy a file, "2" to create a blank file, or "q" to cancel.`)
		}
	}
}

// ensureSourceFileTUI runs the setup flow as a Bubble Tea program and prints
// its outcome after the program has left the alternate screen.
func (p Prompter) ensureSourceFileTUI(ctx harness.Context) (bool, bool, error) {
	model := NewSetupModel(ctx)
	if err := runProgram(model, p.TTY); err != nil {
		return false, false, err
	}
	if model.Err != nil {
		return false, false, model.Err
	}
	if !model.Created {
		fmt.Fprintln(p.Out, "Cancelled. No changes made.")
		return false, false, nil
	}
	if model.From == "" {
		fmt.Fprintf(p.Out, "Created blank instructions file: %s\n", ctx.SourceFile)
	} else {
		fmt.Fprintf(p.Out, "Created %s from %s\n", ctx.SourceFile, model.From)
	}
	return true, true, nil
}

// Confirm asks for approval after showing the plan text. With a terminal the
// review screen applies on a single enter and cancels on esc; without one it
// prompts for an explicit "yes". needsTerminal, when set, becomes an error
// instead of a prompt when the session has no terminal. created reports that
// the setup step created the instructions file this run; it only changes the
// interactive cancellation copy. It returns false when the user cancelled.
func (p Prompter) Confirm(planText, prompt, confirmation, needsTerminal string, created bool) (bool, error) {
	if p.Interactive {
		model := NewConfirmModel(planText)
		if err := runProgram(model, p.TTY); err != nil {
			return false, err
		}
		if model.Confirmed {
			fmt.Fprintln(p.Out, confirmation)
			return true, nil
		}
		fmt.Fprintln(p.Out, cancelledInteractiveLine(created))
		return false, nil
	}

	fmt.Fprintln(p.Out, planText)
	if needsTerminal != "" {
		return false, errors.New(needsTerminal)
	}
	answer, ok := p.Ask(prompt)
	if !ok {
		fmt.Fprintln(p.Out, "Cancelled (no confirmation received). No changes made.")
		return false, nil
	}
	if strings.ToLower(strings.TrimSpace(answer)) == "yes" {
		fmt.Fprintln(p.Out, confirmation)
		return true, nil
	}
	fmt.Fprintln(p.Out, "Cancelled. No changes made.")
	return false, nil
}

// cancelledInteractiveLine is the review screen's cancellation copy. When the
// setup step created the instructions file this run, only the links were left
// unchanged; plain-text output keeps its historical wording.
func cancelledInteractiveLine(created bool) string {
	if created {
		return "Cancelled. No links were changed."
	}
	return "Cancelled. No changes made."
}

// RenderTable renders the harness table without a trailing newline. A
// focusedIndex of -1 means no focused row.
func RenderTable(harnesses []harness.Harness, ctx harness.Context, selected map[string]bool, focusedIndex int) (string, error) {
	headers := []string{"", "Harness", "Installed", "Global instructions"}
	rows := make([][]string, 0, len(harnesses))
	for i, h := range harnesses {
		focused := ""
		if i == focusedIndex {
			focused = ">"
		}
		check := " "
		if selected[h.ID] {
			check = "x"
		}
		installed, err := harness.IsInstalled(h, ctx)
		if err != nil {
			return "", err
		}
		installedCell := "No"
		if installed {
			installedCell = "Yes"
		}
		rows = append(rows, []string{
			focused,
			fmt.Sprintf("[%s] %s", check, h.Name),
			installedCell,
			harness.DisplayPath(h.Target, ctx.Home),
		})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"", "(none detected)", "", ""})
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
		"Global instructions: " + ctx.SourceFile,
		"",
		border("┌", "┬", "┐"),
		renderRow(headers),
		border("├", "┼", "┤"),
	}
	for _, row := range rows {
		lines = append(lines, renderRow(row))
	}
	lines = append(lines, border("└", "┴", "┘"))
	return strings.Join(lines, "\n"), nil
}

func cellWidth(cell string) int {
	return utf8.RuneCountInString(cell)
}

// SelectedFilePath resolves the path a user typed for the copy flow: "~" and
// "~/" expand against home, and relative paths resolve against cwd.
func SelectedFilePath(value, home, cwd string) string {
	trimmed := strings.TrimSpace(value)
	var expanded string
	switch {
	case trimmed == "~":
		expanded = home
	case strings.HasPrefix(trimmed, "~/") || strings.HasPrefix(trimmed, "~"+string(filepath.Separator)):
		expanded = filepath.Join(home, trimmed[2:])
	default:
		expanded = trimmed
	}
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	return filepath.Join(cwd, expanded)
}

// CheckSelectedFile validates the file the user selected for the copy flow.
// The returned errors mirror the prompt messages — "not found", "cannot
// read", "not a regular file" — so the setup loop can re-prompt.
func CheckSelectedFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if harness.MissingPath(err) {
			return fmt.Errorf("not found: %s", path)
		}
		return fmt.Errorf("cannot read: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	return nil
}

// CreateBlankSource creates the canonical instructions file with empty
// contents, creating missing parent directories and refusing to overwrite an
// existing file.
func CreateBlankSource(sourceFile string) error {
	return copyToSourceFile(sourceFile, strings.NewReader(""), 0o666)
}

// CopySourceFile copies selectedPath to the canonical instructions file,
// creating missing parent directories and refusing to overwrite an existing
// file.
func CopySourceFile(sourceFile, selectedPath string) error {
	info, err := os.Stat(selectedPath)
	if err != nil {
		return fmt.Errorf("stat selected file %s: %w", selectedPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", selectedPath)
	}
	source, err := os.Open(selectedPath)
	if err != nil {
		return fmt.Errorf("open selected file %s: %w", selectedPath, err)
	}
	defer source.Close()
	return copyToSourceFile(sourceFile, source, info.Mode().Perm())
}

// copyToSourceFile stages source in a temporary file beside sourceFile and
// publishes it with a hard link, which fails atomically instead of replacing
// an existing path. The temporary name is removed on every outcome.
func copyToSourceFile(sourceFile string, source io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o777); err != nil {
		return fmt.Errorf("create directory for %s: %w", sourceFile, err)
	}
	temp, err := createSourceTemp(sourceFile, perm)
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if _, err := io.Copy(temp, source); err != nil {
		temp.Close()
		return fmt.Errorf("copy to temporary file for %s: %w", sourceFile, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file for %s: %w", sourceFile, err)
	}
	if err := os.Link(tempPath, sourceFile); err != nil {
		return fmt.Errorf("publish %s: %w", sourceFile, err)
	}
	return nil
}

// createSourceTemp creates a temporary file in sourceFile's directory. The
// process umask applies to perm, matching how the canonical file was created
// before it was staged.
func createSourceTemp(sourceFile string, perm os.FileMode) (*os.File, error) {
	dir := filepath.Dir(sourceFile)
	for attempt := 0; attempt < 10; attempt++ {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, fmt.Errorf("generate temporary file name in %s: %w", dir, err)
		}
		path := filepath.Join(dir, "."+filepath.Base(sourceFile)+"-"+hex.EncodeToString(suffix[:]))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("create temporary file for %s: %w", sourceFile, err)
		}
		return file, nil
	}
	return nil, fmt.Errorf("create temporary file for %s: too many name collisions", sourceFile)
}
