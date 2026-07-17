// Package version carries build-time identity injected via -ldflags.
package version

import "fmt"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func String() string {
	return fmt.Sprintf("outpost %s (commit %s, built %s)", Version, Commit, Date)
}
