// Package cli holds the build identity for suspenders. The release
// pipeline injects every component's version at
// <component>/internal/cli.Version, so the var must live here even though
// the cobra commands live under cmd/suspenders/commands.
package cli

// Version is the build version, injected via -ldflags at build time.
var Version = "dev"
