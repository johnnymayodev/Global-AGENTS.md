// Package ui renders the interactive and plain-text user interface: the
// canonical-file setup flow, the selection table, and the confirmation
// prompts.
package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/harness"
)

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
