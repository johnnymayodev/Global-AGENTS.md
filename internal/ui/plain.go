// Package ui renders the interactive and plain-text user interface: the
// canonical-file setup flow, the selection table, and the confirmation
// prompts.
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

// Prompter runs the plain-text interaction path: line prompts over stdin and
// stdout, errors over stderr. Interactive reports that stdin and stdout are
// both terminals; the Bubble Tea models take over those paths once the TUI
// work lands.
type Prompter struct {
	In          *bufio.Reader
	Out         io.Writer
	Err         io.Writer
	Interactive bool
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

// EnsureSourceFile returns true when the canonical instructions file exists,
// running the copy / blank / cancel setup menu when it is missing. It returns
// false when the user cancelled.
func (p Prompter) EnsureSourceFile(ctx harness.Context) (bool, error) {
	exists, err := harness.SourceFileExists(ctx)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	fmt.Fprintf(p.Out, "No shared instructions file found at %s.\n", ctx.SourceFile)
	for {
		choice, ok := p.Ask("Choose: [1] copy an existing instructions file  [2] create a blank AGENTS.md  [q] cancel: ")
		if !ok || strings.ToLower(strings.TrimSpace(choice)) == "q" {
			fmt.Fprintln(p.Out, "Cancelled. No changes made.")
			return false, nil
		}

		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1":
			path, ok := p.Ask("Path to the existing instructions file: ")
			if !ok {
				fmt.Fprintln(p.Out, "Cancelled. No changes made.")
				return false, nil
			}
			selectedPath := SelectedFilePath(path, ctx.Home, ctx.Cwd())
			if err := CheckSelectedFile(selectedPath); err != nil {
				fmt.Fprintln(p.Err, err)
				continue
			}
			if err := CopySourceFile(ctx.SourceFile, selectedPath); err != nil {
				return false, err
			}
			fmt.Fprintf(p.Out, "Created %s from %s\n", ctx.SourceFile, selectedPath)
			return true, nil
		case "2":
			if err := CreateBlankSource(ctx.SourceFile); err != nil {
				return false, err
			}
			fmt.Fprintf(p.Out, "Created blank instructions file: %s\n", ctx.SourceFile)
			return true, nil
		default:
			fmt.Fprintln(p.Out, `Enter "1" to copy a file, "2" to create a blank file, or "q" to cancel.`)
		}
	}
}

// Confirm prompts for an explicit "yes". needsTerminal, when set, becomes an
// error instead of a prompt when the session has no terminal. It returns
// false when the answer was not "yes" or stdin closed before one arrived.
func (p Prompter) Confirm(prompt, confirmation, needsTerminal string) (bool, error) {
	if needsTerminal != "" && !p.Interactive {
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

// ChooseHarnesses runs the interactive picker. Without a terminal it prints
// the table and the non-interactive notice instead. It reports cancelled when
// no selection was made.
func ChooseHarnesses(harnesses []harness.Harness, ctx harness.Context, initial map[string]bool, interactive bool, out io.Writer) (selection map[string]bool, cancelled bool, err error) {
	table, err := RenderTable(harnesses, ctx, initial, -1)
	if err != nil {
		return nil, false, err
	}
	if !interactive {
		fmt.Fprintln(out, table)
		fmt.Fprintln(out, "Interactive selection needs a terminal; no changes made.")
		return nil, true, nil
	}
	if len(harnesses) == 0 {
		fmt.Fprintln(out, table)
		fmt.Fprintln(out, "No supported harnesses were detected. Use `agent-links --all` to show every supported target.")
		return nil, true, nil
	}
	// The Bubble Tea picker model is wired in when the TUI work lands.
	return nil, false, errors.New("interactive selection is not implemented yet")
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
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o777); err != nil {
		return err
	}
	file, err := os.OpenFile(sourceFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return err
	}
	return file.Close()
}

// CopySourceFile copies selectedPath to the canonical instructions file,
// creating missing parent directories and refusing to overwrite an existing
// file.
func CopySourceFile(sourceFile, selectedPath string) error {
	info, err := os.Stat(selectedPath)
	if err != nil {
		return err
	}
	source, err := os.Open(selectedPath)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o777); err != nil {
		return err
	}

	dest, err := os.OpenFile(sourceFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(dest, source); err != nil {
		dest.Close()
		return err
	}
	return dest.Close()
}
