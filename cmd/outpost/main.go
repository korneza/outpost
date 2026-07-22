// Command outpost is the Outpost MCP proxy binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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

// run dispatches args (in os.Args form — args[0] is the program name) to
// the right subcommand and returns a process exit code. Extracted from
// main so the CLI's dispatch logic — usage/error paths, the "run"
// subcommand's -- separator parsing — is unit-testable without needing
// to fork a real process or mutate os.Args.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[1] {
	case "version":
		fmt.Fprintln(stdout, version.String())
		return 0
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		configPath := fs.String("config", "outpost.yaml", "path to Outpost config file")
		if err := fs.Parse(args[2:]); err != nil {
			return 2
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return runServe(ctx, *configPath, stdout, stderr)
	case "run":
		cmdArgs := args[2:]
		sep := -1
		for i, a := range cmdArgs {
			if a == "--" {
				sep = i
				break
			}
		}
		if sep < 0 || sep == len(cmdArgs)-1 {
			fmt.Fprintln(stderr, "outpost: run requires -- <server-cmd> [args...]")
			return 2
		}
		// sep+2 can equal len(cmdArgs) exactly (when "--" is followed by
		// exactly one arg) — cmdArgs[len(cmdArgs):] is valid Go (an empty
		// slice, not a panic); the sep == len(cmdArgs)-1 check above
		// already rules out sep+1 being out of range. gosec's static
		// analysis doesn't reason about that guard — false positive,
		// covered by TestRunSubcommandDispatchesToStdioWrapper.
		command, childArgs := cmdArgs[sep+1], cmdArgs[sep+2:] //nolint:gosec
		if err := runStdioWrapper(runConfig{stateDB: "outpost.db"}, stdin, stdout, command, childArgs); err != nil {
			fmt.Fprintf(stderr, "outpost run: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "outpost: unknown command %q\n\n%s", args[1], usage)
		return 2
	}
}

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
