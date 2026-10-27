// Package harness resolves the supported AI coding harnesses and their
// global instructions and skills targets from the process environment.
package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Harness is one supported AI coding harness and where it reads its global
// instructions and skills from.
type Harness struct {
	ID          string
	Name        string
	Target      string
	SkillsDir   string
	Executables []string
	InstallDirs []string
}

// Context is the resolved environment shared by every command: the home
// directory, the canonical shared instructions file and skills directory,
// the Codex home (needed for the AGENTS.override.md note) and every supported
// harness.
type Context struct {
	Home            string
	SourceFile      string
	SourceSkillsDir string
	CodexHome       string
	Harnesses       []Harness

	path string // PATH captured when the context was built
	cwd  string // working directory captured when the context was built
}

// State is how the filesystem entry at a harness target relates to the
// canonical shared instructions file.
type State string

const (
	// Managed marks a symlink pointing at the canonical instructions file.
	Managed State = "managed"
	// Symlink marks a symlink pointing anywhere else.
	Symlink State = "symlink"
	// Path marks a regular file or directory.
	Path State = "path"
	// Absent marks a missing entry.
	Absent State = "absent"
)

// LookupEnv mirrors os.LookupEnv: it reports the value of an environment
// variable and whether the variable is present at all.
type LookupEnv func(key string) (string, bool)

// BuildContext resolves every path from the given environment lookup and
// working directory. Both are injected so tests never touch the real
// environment or working directory.
func BuildContext(lookup LookupEnv, cwd string) (Context, error) {
	home, _ := lookup("HOME")
	if home == "" {
		return Context{}, errors.New("HOME must be set")
	}
	if !filepath.IsAbs(home) {
		return Context{}, errors.New("HOME must be an absolute path")
	}

	envOr := func(key, fallback string) string {
		if value, _ := lookup(key); value != "" {
			return value
		}
		return fallback
	}

	sourceFile := filepath.Join(home, ".config", "agents", "AGENTS.md")
	sourceSkillsDir := filepath.Join(home, ".config", "agents", "skills")
	codexHome := absoluteFrom(envOr("CODEX_HOME", filepath.Join(home, ".codex")), cwd)
	hermesHome := absoluteFrom(envOr("HERMES_HOME", filepath.Join(home, ".hermes")), cwd)

	ompConfigRoot := absoluteFrom(envOr("PI_CONFIG_DIR", ".omp"), home)

	// OMP_PROFILE present in the environment wins over PI_PROFILE even when it
	// holds an empty value.
	ompProfile := ""
	if profile, present := lookup("OMP_PROFILE"); present {
		ompProfile = profile
	} else {
		ompProfile, _ = lookup("PI_PROFILE")
	}

	var ompAgentDir string
	switch {
	case ompProfile != "":
		ompAgentDir = filepath.Join(ompConfigRoot, "profiles", ompProfile, "agent")
	default:
		if agentDir, _ := lookup("PI_CODING_AGENT_DIR"); agentDir != "" {
			ompAgentDir = absoluteFrom(agentDir, cwd)
		} else {
			ompAgentDir = filepath.Join(ompConfigRoot, "agent")
		}
	}

	harnesses := []Harness{
		{
			ID:          "opencode",
			Name:        "OpenCode",
			Target:      filepath.Join(home, ".config", "opencode", "AGENTS.md"),
			SkillsDir:   filepath.Join(home, ".config", "opencode", "skills"),
			Executables: []string{"opencode"},
			InstallDirs: []string{filepath.Join(home, ".config", "opencode")},
		},
		{
			ID:          "codex",
			Name:        "Codex",
			Target:      filepath.Join(codexHome, "AGENTS.md"),
			SkillsDir:   filepath.Join(home, ".agents", "skills"),
			Executables: []string{"codex"},
			InstallDirs: []string{codexHome},
		},
		{
			ID:          "pi",
			Name:        "Pi",
			Target:      filepath.Join(home, ".pi", "agent", "AGENTS.md"),
			SkillsDir:   filepath.Join(home, ".pi", "agent", "skills"),
			Executables: []string{"pi"},
			InstallDirs: []string{filepath.Join(home, ".pi", "agent")},
		},
		{
			ID:          "omp",
			Name:        "OMP",
			Target:      filepath.Join(ompAgentDir, "AGENTS.md"),
			SkillsDir:   filepath.Join(ompAgentDir, "skills"),
			Executables: []string{"omp", "oh-my-pi"},
			InstallDirs: []string{ompAgentDir, ompConfigRoot},
		},
		{
			ID:          "hermes",
			Name:        "Hermes Agent",
			Target:      filepath.Join(hermesHome, "AGENTS.md"),
			SkillsDir:   filepath.Join(hermesHome, "skills"),
			Executables: []string{"hermes"},
			InstallDirs: []string{hermesHome},
		},
		{
			ID:          "claude",
			Name:        "Claude Code",
			Target:      filepath.Join(home, ".claude", "CLAUDE.md"),
			SkillsDir:   filepath.Join(home, ".claude", "skills"),
			Executables: []string{"claude"},
			InstallDirs: []string{filepath.Join(home, ".claude")},
		},
	}

	path, _ := lookup("PATH")
	return Context{
		Home:            home,
		SourceFile:      sourceFile,
		SourceSkillsDir: sourceSkillsDir,
		CodexHome:       codexHome,
		Harnesses:       harnesses,
		path:            path,
		cwd:             cwd,
	}, nil
}

// Cwd returns the working directory captured when the context was built.
func (c Context) Cwd() string { return c.cwd }

// MissingPath reports whether err means the path does not exist. The
// TypeScript implementation treats ENOENT and ENOTDIR as missing; Go maps
// only ENOENT to fs.ErrNotExist.
func MissingPath(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// SourceFileExists reports whether the canonical instructions file exists. A
// broken symlink or a non-regular file at that path is an error.
func SourceFileExists(ctx Context) (bool, error) {
	info, err := os.Lstat(ctx.SourceFile)
	if err != nil {
		if MissingPath(err) {
			return false, nil
		}
		return false, err
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Stat(ctx.SourceFile)
		if err != nil {
			if MissingPath(err) {
				return false, fmt.Errorf("canonical instructions path is a broken symlink: %s; no changes made", ctx.SourceFile)
			}
			return false, err
		}
		if !target.Mode().IsRegular() {
			return false, fmt.Errorf("canonical instructions path is not a regular file: %s; no changes made", ctx.SourceFile)
		}
		return true, nil
	}

	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("canonical instructions path is not a regular file: %s; no changes made", ctx.SourceFile)
	}
	return true, nil
}

// AssertSourceFile errors when no canonical instructions file is present.
func AssertSourceFile(ctx Context) error {
	exists, err := SourceFileExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("missing canonical instructions file: %s; no changes made", ctx.SourceFile)
	}
	return nil
}

// TargetState classifies the filesystem entry at target relative to source,
// which may be a file or a directory.
func TargetState(target, source string) (State, error) {
	info, err := os.Lstat(target)
	if err != nil {
		if MissingPath(err) {
			return Absent, nil
		}
		return "", err
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		dest, err := os.Readlink(target)
		if err != nil {
			return "", err
		}
		if dest == source {
			return Managed, nil
		}
		return Symlink, nil
	}
	return Path, nil
}

// HasExecutable reports whether name resolves to an executable file on
// searchPath. An empty PATH entry (including an unset PATH) searches cwd.
func HasExecutable(name, searchPath, cwd string) bool {
	entries := filepath.SplitList(searchPath)
	if len(entries) == 0 {
		entries = []string{""}
	}
	for _, entry := range entries {
		if entry == "" {
			entry = cwd
		}
		candidate := filepath.Join(entry, name)
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(cwd, candidate)
		}
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		return true
	}
	return false
}

func isDirectory(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if MissingPath(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// IsInstalled reports whether harness looks installed: its target is already
// managed, one of its executables is on PATH, or one of its install
// directories exists.
func IsInstalled(h Harness, ctx Context) (bool, error) {
	state, err := TargetState(h.Target, ctx.SourceFile)
	if err != nil {
		return false, err
	}
	if state == Managed {
		return true, nil
	}
	for _, executable := range h.Executables {
		if HasExecutable(executable, ctx.path, ctx.cwd) {
			return true, nil
		}
	}
	for _, dir := range h.InstallDirs {
		ok, err := isDirectory(dir)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// DisplayPath renders path relative to home with a ~ prefix, or unchanged
// when it is not below home.
func DisplayPath(path, home string) string {
	rel, err := filepath.Rel(home, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "~"
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "~/" + rel
	}
	return path
}

// Visible returns the harnesses to show: all of them when showAll is set,
// otherwise only the installed ones.
func Visible(ctx Context, showAll bool) ([]Harness, error) {
	if showAll {
		return ctx.Harnesses, nil
	}
	var visible []Harness
	for _, h := range ctx.Harnesses {
		installed, err := IsInstalled(h, ctx)
		if err != nil {
			return nil, err
		}
		if installed {
			visible = append(visible, h)
		}
	}
	return visible, nil
}

// VisibleSkills returns the harnesses to show in the skills matrix. In
// detected mode a harness counts as present when it is installed or when its
// skills directory already holds a link managed by this tool.
func VisibleSkills(ctx Context, showAll bool) ([]Harness, error) {
	if showAll {
		return ctx.Harnesses, nil
	}
	var visible []Harness
	for _, h := range ctx.Harnesses {
		installed, err := IsInstalled(h, ctx)
		if err != nil {
			return nil, err
		}
		if !installed {
			installed, err = hasManagedSkillLink(h, ctx)
			if err != nil {
				return nil, err
			}
		}
		if installed {
			visible = append(visible, h)
		}
	}
	return visible, nil
}

// hasManagedSkillLink reports whether h's skills directory contains a symlink
// pointing at a skill directory under the canonical shared skills root.
func hasManagedSkillLink(h Harness, ctx Context) (bool, error) {
	if h.SkillsDir == "" {
		return false, nil
	}
	entries, err := os.ReadDir(h.SkillsDir)
	if err != nil {
		if MissingPath(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink == 0 {
			continue
		}
		target := filepath.Join(h.SkillsDir, entry.Name())
		dest, err := os.Readlink(target)
		if err != nil {
			if MissingPath(err) {
				continue
			}
			return false, err
		}
		if dest == filepath.Join(ctx.SourceSkillsDir, entry.Name()) {
			return true, nil
		}
	}
	return false, nil
}

// SkillUnavailableReason reports why a harness cannot accept a skill name, or
// "" when the pair is allowed. Only Claude Code reserves skill names.
func SkillUnavailableReason(h Harness, name string) string {
	if h.ID != "claude" {
		return ""
	}
	lower := strings.ToLower(name)
	switch {
	case lower == "synced":
		return `Claude reserves the name "synced" for skills synced from claude.ai`
	case lower == "anthropic-skills" || strings.HasPrefix(lower, "anthropic-skills:"):
		return `Claude reserves the "anthropic-skills" namespace for synced skills`
	}
	return ""
}

// absoluteFrom resolves value against base when it is not already absolute,
// matching node:path resolve.
func absoluteFrom(value, base string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(base, value)
}
