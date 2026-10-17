// Package cli owns argument dispatch and the top-level error and exit-code
// wrapping. Full dispatch lands with the CLI port; the domain packages under
// internal/ are already in place.
package cli

import (
	"fmt"
	"os"
)

// Run executes the CLI with the given arguments and returns the process exit
// code.
func Run(args []string) int {
	fmt.Fprintln(os.Stderr, "error: agent-links is not wired up yet")
	return 2
}
