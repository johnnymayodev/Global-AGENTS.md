// Command agent-links links one shared AGENTS.md into supported AI coding
// harnesses.
package main

import (
	"os"

	"github.com/johnnymayodev/Global-AGENTS.md/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
