// Package version holds build-time metadata for XXC The Cleaners.
package version

import (
	"fmt"
	"runtime"
)

// These are overridden at build time via -ldflags (see Makefile).
var (
	// Name is the program's short name.
	Name = "xxc-cleaners"
	// Pretty is the program's display name.
	Pretty = "XXC The Cleaners"
	// Version is the semantic version, injected by the build.
	Version = "0.0.0-dev"
	// Commit is the git commit hash, injected by the build.
	Commit = "unknown"
	// BuildDate is the build timestamp, injected by the build.
	BuildDate = "unknown"
)

const (
	// Author is the project author.
	Author = "Kawaiipantsu"
	// Org is the community behind the project.
	Org = "THUGS(red) hacking community"
	// URL is the project home.
	URL = "https://thugs.red"
)

// String returns a one-line version summary.
func String() string {
	return fmt.Sprintf("%s (%s) %s", Pretty, Name, Version)
}

// Long returns a detailed multi-line version block.
func Long() string {
	return fmt.Sprintf(
		"%s\n  version : %s\n  commit  : %s\n  built   : %s\n  go      : %s\n  platform: %s/%s",
		String(), Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}

// About returns the --about banner.
func About() string {
	return fmt.Sprintf(`%s  v%s

  "When things get messy, you call in The Cleaners."

  A retention & cleanup tool for Linux servers and workstations.
  It hunts down caches, stale logs, core dumps, build artifacts and
  other digital grime — then takes out the trash (only when you say so).

  Author : %s
  Crew   : %s
  Home   : %s

  Run with --help for options, or --interactive for the visual mode.`,
		Pretty, Version, Author, Org, URL)
}
