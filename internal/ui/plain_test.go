package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectedFilePath(t *testing.T) {
	home := "/Users/johnny"
	cwd := "/work/dir"
	cases := []struct {
		value string
		want  string
	}{
		{value: "~", want: "/Users/johnny"},
		{value: "~/notes/AGENTS.md", want: "/Users/johnny/notes/AGENTS.md"},
		{value: "  ~/notes/AGENTS.md  ", want: "/Users/johnny/notes/AGENTS.md"},
		{value: "notes/AGENTS.md", want: "/work/dir/notes/AGENTS.md"},
		{value: "/etc/AGENTS.md", want: "/etc/AGENTS.md"},
		{value: "/etc/../AGENTS.md", want: "/AGENTS.md"},
		{value: "", want: "/work/dir"},
		{value: "~~/notes", want: "/work/dir/~~/notes"},
	}
	for _, tc := range cases {
		if got := SelectedFilePath(tc.value, home, cwd); got != tc.want {
			t.Errorf("SelectedFilePath(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestCheckSelectedFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing path", func(t *testing.T) {
		path := filepath.Join(dir, "missing.md")
		want := "not found: " + path
		if err := CheckSelectedFile(path); err == nil || err.Error() != want {
			t.Fatalf("CheckSelectedFile error = %v, want %q", err, want)
		}
	})

	t.Run("not found below a file", func(t *testing.T) {
		file := filepath.Join(dir, "plain-file")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(file, "AGENTS.md")
		want := "not found: " + path
		if err := CheckSelectedFile(path); err == nil || err.Error() != want {
			t.Fatalf("CheckSelectedFile error = %v, want %q", err, want)
		}
	})

	t.Run("directory", func(t *testing.T) {
		path := filepath.Join(dir, "subdir")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		want := "not a regular file: " + path
		if err := CheckSelectedFile(path); err == nil || err.Error() != want {
			t.Fatalf("CheckSelectedFile error = %v, want %q", err, want)
		}
	})

	t.Run("regular file", func(t *testing.T) {
		path := filepath.Join(dir, "AGENTS.md")
		if err := os.WriteFile(path, []byte("shared"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := CheckSelectedFile(path); err != nil {
			t.Fatalf("CheckSelectedFile: %v", err)
		}
	})
}

func TestCreateBlankSource(t *testing.T) {
	t.Run("creates missing directories and an empty file", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), ".config", "agents", "AGENTS.md")
		if err := CreateBlankSource(source); err != nil {
			t.Fatalf("CreateBlankSource: %v", err)
		}
		contents, err := os.ReadFile(source)
		if err != nil || len(contents) != 0 {
			t.Fatalf("source contents = %q, %v; want empty", contents, err)
		}
	})

	t.Run("refuses to overwrite an existing file", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), "AGENTS.md")
		if err := os.WriteFile(source, []byte("keep me"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := CreateBlankSource(source); err == nil {
			t.Fatal("CreateBlankSource unexpectedly succeeded")
		}
		contents, err := os.ReadFile(source)
		if err != nil || string(contents) != "keep me" {
			t.Fatalf("source contents = %q, %v; want unchanged", contents, err)
		}
	})
}

func TestCopySourceFile(t *testing.T) {
	t.Run("copies contents and creates missing directories", func(t *testing.T) {
		selected := filepath.Join(t.TempDir(), "notes", "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(selected, []byte("# Shared\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		source := filepath.Join(t.TempDir(), ".config", "agents", "AGENTS.md")
		if err := CopySourceFile(source, selected); err != nil {
			t.Fatalf("CopySourceFile: %v", err)
		}
		contents, err := os.ReadFile(source)
		if err != nil || string(contents) != "# Shared\n" {
			t.Fatalf("source contents = %q, %v", contents, err)
		}
	})

	t.Run("refuses to overwrite an existing file", func(t *testing.T) {
		selected := filepath.Join(t.TempDir(), "AGENTS.md")
		if err := os.WriteFile(selected, []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(t.TempDir(), "AGENTS.md")
		if err := os.WriteFile(source, []byte("keep me"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := CopySourceFile(source, selected); err == nil {
			t.Fatal("CopySourceFile unexpectedly succeeded")
		}
		contents, err := os.ReadFile(source)
		if err != nil || string(contents) != "keep me" {
			t.Fatalf("source contents = %q, %v; want unchanged", contents, err)
		}
	})

	t.Run("missing selected file", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), "AGENTS.md")
		if err := CopySourceFile(source, filepath.Join(t.TempDir(), "missing.md")); err == nil {
			t.Fatal("CopySourceFile unexpectedly succeeded")
		}
		if _, err := os.Lstat(source); err == nil {
			t.Error("source file should not have been created")
		}
	})
}
