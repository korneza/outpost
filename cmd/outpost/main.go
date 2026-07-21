// Command outpost is the Outpost MCP proxy binary.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/korneza/outpost/internal/version"
)

const usage = `Outpost — MCP proxy for agent reliability and security visibility.

Usage:
  outpost <command>

Commands:
  serve      Run the proxy
  run        Run as a stdio wrapper around a child MCP server: outpost run -- <server-cmd> [args...]
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
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		configPath := fs.String("config", "outpost.yaml", "path to Outpost config file")
		if err := fs.Parse(os.Args[2:]); err != nil {
			os.Exit(2)
		}
		os.Exit(runServe(*configPath, os.Stdout, os.Stderr))
	case "run":
		args := os.Args[2:]
		sep := -1
		for i, a := range args {
			if a == "--" {
				sep = i
				break
			}
		}
		if sep < 0 || sep == len(args)-1 {
			fmt.Fprintln(os.Stderr, "outpost: run requires -- <server-cmd> [args...]")
			os.Exit(2)
		}
		command, cmdArgs := args[sep+1], args[sep+2:]
		if err := runStdioWrapper(runConfig{stateDB: "outpost.db"}, os.Stdin, os.Stdout, command, cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "outpost run: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "outpost: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}
