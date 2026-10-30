package cli

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

// Version is the release version stamped at build time with
//
//	go build -ldflags "-X github.com/johnnymayodev/Global-AGENTS.md/internal/cli.Version=0.5.2"
//
// build.sh reads the value from run.sh's version= line. It stays empty for a
// local build, where the module version (if any) is used instead.
var Version = ""

// printVersion writes the version line, e.g. "agent-links v0.5.2".
func printVersion(out io.Writer) {
	fmt.Fprintf(out, "agent-links %s\n", displayVersion())
}

// displayVersion resolves the version to report from the build stamp and, when
// there is none, from the module version recorded by go install.
func displayVersion() string {
	return formatVersion(Version, moduleVersion())
}

// moduleVersion reports the released module version, which `go install
// ...@latest` records. A build from a working copy carries VCS settings and no
// released version, so it reports "" and the caller falls back to "dev".
func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return ""
		}
	}
	return info.Main.Version
}

// formatVersion normalizes a stamped or module version for display. A single
// leading "v" is dropped so output is consistent, and a missing or development
// version becomes "dev".
func formatVersion(stamped, module string) string {
	version := strings.TrimSpace(stamped)
	if version == "" {
		version = strings.TrimSpace(module)
	}
	version = strings.TrimPrefix(version, "v")
	if version == "" || version == "(devel)" {
		return "dev"
	}
	return "v" + version
}
