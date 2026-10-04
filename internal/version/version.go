// Package version holds build metadata injected at link time by GoReleaser
// (see .goreleaser.yaml, ldflags).
package version

// These defaults are overridden via -ldflags -X in release builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Info returns the build version, commit hash and build date.
func Info() (ver, cmt, builtAt string) {
	return version, commit, date
}

// String returns a human-readable one-line version string.
func String() string {
	return version + " (commit " + commit + ", built " + date + ")"
}
