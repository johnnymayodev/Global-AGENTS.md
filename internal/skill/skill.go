// Package skill discovers portable Agent Skills in the canonical shared
// skills directory. Discovery checks the directory shape only; it does not
// parse or validate SKILL.md frontmatter, which remains each harness's job.
package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode"
)

// Skill is one shared skill directory.
type Skill struct {
	// Name is the skill directory's basename and its destination name.
	Name string
	// SourceDir is the path to the skill directory in the canonical root.
	SourceDir string
}

// Discover returns the valid immediate children of root: directories, or
// symlinks to directories, that contain a regular SKILL.md. A missing or
// empty root is an empty result rather than an error. A root that is not a
// directory, a broken root symlink, and permission or I/O failures are errors.
func Discover(root string) ([]Skill, error) {
	info, err := os.Lstat(root)
	if err != nil {
		if missing(err) {
			return nil, nil
		}
		return nil, err
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Stat(root)
		if err != nil {
			if missing(err) {
				return nil, fmt.Errorf("shared skills path is a broken symlink: %s", root)
			}
			return nil, err
		}
		if !target.IsDir() {
			return nil, fmt.Errorf("shared skills path is not a directory: %s", root)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("shared skills path is not a directory: %s", root)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if missing(err) {
			return nil, nil
		}
		return nil, err
	}

	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		dir := filepath.Join(root, name)

		info, err := os.Stat(dir)
		if err != nil {
			if missing(err) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}

		info, err = os.Stat(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			if missing(err) {
				continue
			}
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}

		if err := ValidateName(name); err != nil {
			return nil, fmt.Errorf("invalid skill directory name: %q", name)
		}

		skills = append(skills, Skill{Name: name, SourceDir: dir})
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// Find returns one named, discoverable skill from root.
func Find(root, name string) (Skill, error) {
	if err := ValidateName(name); err != nil {
		return Skill{}, err
	}
	skills, err := Discover(root)
	if err != nil {
		return Skill{}, err
	}
	for _, s := range skills {
		if s.Name == name {
			return s, nil
		}
	}
	return Skill{}, fmt.Errorf("skill not found: %s", name)
}

// ValidateName accepts one plain path component and rejects anything that
// could escape a skills root.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("skill name must not be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid skill name: %q", name)
	}
	if filepath.IsAbs(name) || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid skill name: %q", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("invalid skill name: %q", name)
		}
	}
	return nil
}

func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}
