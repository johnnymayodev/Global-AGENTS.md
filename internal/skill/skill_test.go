package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeSkill(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	writeFile(t, filepath.Join(dir, "SKILL.md"), "# "+name+"\n")
	return dir
}

func TestDiscover(t *testing.T) {
	t.Run("missing root is empty", func(t *testing.T) {
		skills, err := Discover(filepath.Join(t.TempDir(), "missing"))
		if err != nil || len(skills) != 0 {
			t.Fatalf("Discover = %v, %v; want empty", skills, err)
		}
	})

	t.Run("empty root is empty", func(t *testing.T) {
		root := t.TempDir()
		skills, err := Discover(root)
		if err != nil || len(skills) != 0 {
			t.Fatalf("Discover = %v, %v; want empty", skills, err)
		}
	})

	t.Run("root that is a file is an error", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "skills")
		writeFile(t, root, "not a directory")
		_, err := Discover(root)
		want := "shared skills path is not a directory: " + root
		if err == nil || err.Error() != want {
			t.Fatalf("Discover error = %v, want %q", err, want)
		}
	})

	t.Run("broken root symlink is an error", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "skills")
		if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), root); err != nil {
			t.Fatal(err)
		}
		_, err := Discover(root)
		want := "shared skills path is a broken symlink: " + root
		if err == nil || err.Error() != want {
			t.Fatalf("Discover error = %v, want %q", err, want)
		}
	})

	t.Run("ignores non-qualifying children and only reads immediate children", func(t *testing.T) {
		root := t.TempDir()
		makeSkill(t, root, "alpha")
		writeFile(t, filepath.Join(root, "loose.md"), "# not a skill\n")
		if err := os.MkdirAll(filepath.Join(root, "no-skill"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "category", "nested", "SKILL.md"), 0o755); err != nil {
			t.Fatal(err)
		}

		skills, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(skills) != 1 || skills[0].Name != "alpha" {
			t.Fatalf("skills = %v, want only alpha", skills)
		}
	})

	t.Run("permission failure at the root is an error", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root; permission checks do not apply")
		}
		root := t.TempDir()
		if err := os.Chmod(root, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(root, 0o755) })
		if _, err := Discover(root); err == nil {
			t.Fatal("Discover unexpectedly treated an unreadable root as empty")
		}
	})

	t.Run("root symlink to a directory is accepted", func(t *testing.T) {
		real := t.TempDir()
		makeSkill(t, real, "alpha")
		root := filepath.Join(t.TempDir(), "skills")
		if err := os.Symlink(real, root); err != nil {
			t.Fatal(err)
		}
		skills, err := Discover(root)
		if err != nil || len(skills) != 1 || skills[0].Name != "alpha" || skills[0].SourceDir != filepath.Join(root, "alpha") {
			t.Fatalf("Discover = %v, %v; want alpha below the symlinked root", skills, err)
		}
	})

	t.Run("rejects a skill directory with control characters", func(t *testing.T) {
		root := t.TempDir()
		name := "evil\x1b[31m\nname"
		makeSkill(t, root, name)

		_, err := Discover(root)
		want := fmt.Sprintf("invalid skill directory name: %q", name)
		if err == nil || err.Error() != want {
			t.Fatalf("Discover error = %v, want %q", err, want)
		}
	})

	t.Run("sorts by name", func(t *testing.T) {
		root := t.TempDir()
		makeSkill(t, root, "zeta")
		makeSkill(t, root, "alpha")
		makeSkill(t, root, "middle")

		skills, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		var names []string
		for _, s := range skills {
			names = append(names, s.Name)
		}
		if got, want := strings.Join(names, ","), "alpha,middle,zeta"; got != want {
			t.Fatalf("names = %q, want %q", got, want)
		}
	})

	t.Run("accepts a symlinked skill directory and keeps its configured path", func(t *testing.T) {
		root := t.TempDir()
		real := makeSkill(t, t.TempDir(), "real")
		link := filepath.Join(root, "linked")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}

		skills, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(skills) != 1 || skills[0].Name != "linked" || skills[0].SourceDir != link {
			t.Fatalf("skills = %v, want linked -> %s", skills, link)
		}
	})
}

func TestFind(t *testing.T) {
	root := t.TempDir()
	dir := makeSkill(t, root, "git-release")

	s, err := Find(root, "git-release")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if s.Name != "git-release" || s.SourceDir != dir {
		t.Fatalf("Find = %+v", s)
	}

	if _, err := Find(root, "missing"); err == nil || err.Error() != "skill not found: missing" {
		t.Fatalf("Find missing error = %v", err)
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"git-release", "pdf_tools", "skill.name", "A", "..hidden", "技能"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
	invalid := []string{"", ".", "..", "/abs", "a/b", `a\b`, "../escape", "\x1b[31m", "a\nb", "a\tb"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", name)
		}
	}
}

// FuzzValidateName checks the safety contract behind ValidateName: a name is
// either rejected or is exactly one ordinary path component that cannot escape
// a skills root or disrupt terminal output. Seeds mirror TestValidateName.
func FuzzValidateName(f *testing.F) {
	for _, name := range []string{"git-release", "pdf_tools", "skill.name", "A", "..hidden", "技能"} {
		f.Add(name)
	}
	for _, name := range []string{"", ".", "..", "/abs", "a/b", `a\b`, "../escape", "\x1b[31m", "a\nb", "a\tb"} {
		f.Add(name)
	}

	f.Fuzz(func(t *testing.T, name string) {
		if err := ValidateName(name); err != nil {
			return
		}
		if name == "" || name == "." || name == ".." {
			t.Fatalf("ValidateName(%q) accepted a special path component", name)
		}
		if strings.ContainsAny(name, `/\`) {
			t.Fatalf("ValidateName(%q) accepted a path separator", name)
		}
		if filepath.IsAbs(name) || filepath.Base(name) != name {
			t.Fatalf("ValidateName(%q) accepted more than one path component", name)
		}
		if strings.ContainsRune(name, 0) {
			t.Fatalf("ValidateName(%q) accepted NUL", name)
		}
		for _, r := range name {
			if unicode.IsControl(r) {
				t.Fatalf("ValidateName(%q) accepted control character %U", name, r)
			}
		}
	})
}
