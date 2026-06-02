package version

import (
	"fmt"
	"runtime"
)

// These values are injected at build time via -ldflags. See the Makefile and
// .goreleaser.yaml. Defaults apply for `go run` / `go install` builds.
var (
	// Version is the release version (e.g. "v0.1.0"), set by goreleaser.
	Version = "dev"
	// Commit is the short git SHA of the build.
	Commit = "dev"
	// Date is the build timestamp (RFC3339), set by goreleaser.
	Date = "unknown"
)

// String returns a human-readable, single-line version summary.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s)", Version, Commit, Date, runtime.Version())
}
