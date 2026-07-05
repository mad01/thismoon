package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version of t-man
	Version = "0.1.0"

	// GitCommit is the git commit hash (set during build)
	GitCommit = "unknown"

	// BuildDate is the build date (set during build)
	BuildDate = "unknown"

	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
)

// Info returns formatted version information
func Info() string {
	return fmt.Sprintf("t-man version %s\nGit commit: %s\nBuild date: %s\nGo version: %s",
		Version, GitCommit, BuildDate, GoVersion)
}

// Short returns a short version string
func Short() string {
	return Version
}

// Full returns the full version string with git commit
func Full() string {
	if GitCommit != "unknown" {
		return fmt.Sprintf("%s-%s", Version, GitCommit)
	}
	return Version
}
