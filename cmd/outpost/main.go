// Command outpost is the Outpost MCP proxy binary.
package main

import (
	"fmt"
	"os"

	"github.com/korneza/outpost/internal/version"
)

const usage = `Outpost — MCP proxy for agent reliability and security visibility.

Usage:
  outpost <command>

Commands:
  serve      Run the proxy (not yet implemented)
  version    Print version information
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version.String())
	case "serve":
		fmt.Fprintln(os.Stderr, "outpost serve: not implemented yet — proxy core lands in the next change set")
		os.Exit(2)
	default:
		fmt.Fprintf(os.Stderr, "outpost: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
