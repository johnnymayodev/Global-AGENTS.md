package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name    string
		stamped string
		module  string
		want    string
	}{
		{name: "stamped", stamped: "0.5.2", want: "v0.5.2"},
		{name: "stamped with v", stamped: "v0.5.2", want: "v0.5.2"},
		{name: "stamped wins over module", stamped: "0.6.0", module: "v0.5.2", want: "v0.6.0"},
		{name: "module fallback", module: "v1.2.3", want: "v1.2.3"},
		{name: "development", module: "(devel)", want: "dev"},
		{name: "empty", want: "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatVersion(tt.stamped, tt.module); got != tt.want {
				t.Errorf("formatVersion(%q, %q) = %q, want %q", tt.stamped, tt.module, got, tt.want)
			}
		})
	}
}

func TestVersionWithoutHome(t *testing.T) {
	previous := Version
	Version = "0.5.2"
	defer func() { Version = previous }()

	for _, arg := range []string{"--version", "-v"} {
		var stdout, stderr bytes.Buffer
		code := RunWith([]string{arg}, Options{
			LookupEnv: func(string) (string, bool) { return "", false },
			Cwd:       t.TempDir(),
			Stdin:     strings.NewReader(""),
			Stdout:    &stdout,
			Stderr:    &stderr,
		})
		if code != 0 {
			t.Errorf("%s exit code = %d, want 0", arg, code)
		}
		if want := "agent-links v0.5.2\n"; stdout.String() != want {
			t.Errorf("%s stdout = %q, want %q", arg, stdout.String(), want)
		}
		if stderr.Len() != 0 {
			t.Errorf("%s stderr = %q, want empty", arg, stderr.String())
		}
	}
}
