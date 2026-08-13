// Package buildinfo reports the build metadata a thismoon component embeds:
// the short git sha it was built from, the full commit, the component release
// tag, and when it was built. Component Makefiles inject the package variables
// with -ldflags (see buildinfo.mk at the repo root); a binary built without
// them falls back to the vcs stamps the Go toolchain records, which is what a
// `go install`ed binary carries.
//
// The JSON object is the cross-tool contract every component serves from
// GET /version and prints for `<binary> version -o json`: four keys, always
// present, "" for anything unknown. Plain `<binary> version` stays the bare
// version token that status and ralph probe for.
package buildinfo

import (
	"encoding/json"
	"regexp"
	"runtime/debug"
	"strings"
)

// Build metadata injected at link time with -ldflags. The defaults describe a
// plain `go build`; buildinfo.mk and the release workflow set all four.
var (
	// Version is the short git sha, or "<tag>-<sha>" for a release build.
	Version = devVersion
	// Commit is the full 40-character git sha.
	Commit = ""
	// Tag is the component release tag, e.g. "keep/v0.4.0".
	Tag = ""
	// BuildTime is the UTC build timestamp in RFC 3339.
	BuildTime = ""
)

// devVersion is the Version placeholder for a build with no metadata injected.
const devVersion = "dev"

// shortSHALen is the width `git rev-parse --short` prints by default, so a
// version derived from a vcs stamp looks like an injected one.
const shortSHALen = 7

// Info is the build metadata object a component reports. Every field is
// serialized even when empty: consumers parse a fixed four-key shape.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Tag       string `json:"tag"`
	BuildTime string `json:"build_time"`
}

// Get returns the linked-in build metadata, filling whatever -ldflags left
// unset from the toolchain's vcs stamps.
func Get() Info {
	i := Info{Version: Version, Commit: Commit, Tag: Tag, BuildTime: BuildTime}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return i
	}
	return i.fallback(bi)
}

// fallback fills empty fields from bi's vcs stamps and module version. It never
// overwrites a value -ldflags provided; Version counts as unset while it still
// holds the "dev" placeholder. vcs.time is the commit timestamp rather than the
// moment of the build — the closest stand-in the toolchain records.
func (i Info) fallback(bi *debug.BuildInfo) Info {
	settings := make(map[string]string, len(bi.Settings))
	for _, s := range bi.Settings {
		settings[s.Key] = s.Value
	}
	if rev := settings["vcs.revision"]; rev != "" {
		if i.Commit == "" {
			i.Commit = rev
		}
		if i.Version == "" || i.Version == devVersion {
			i.Version = shortSHA(rev)
		}
	}
	if i.BuildTime == "" {
		i.BuildTime = settings["vcs.time"]
	}
	if i.Tag == "" && isReleaseVersion(bi.Main.Version) {
		i.Tag = bi.Main.Version
	}
	return i
}

// pseudoVersionTailRe matches the "<14-digit timestamp>-<12 hex>" tail the
// toolchain appends when it derives a module version from a commit rather than
// a tag (both the v0.0.0-<stamp> and the vX.Y.Z-0.<stamp> forms). Since Go 1.24
// an untagged build reports such a pseudo-version instead of "(devel)", and a
// commit is already the Commit field, not a release tag.
var pseudoVersionTailRe = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}(\+[0-9A-Za-z.-]+)?$`)

// isReleaseVersion reports whether v is a module version that came from a real
// tag, as opposed to "(devel)" or a commit-derived pseudo-version.
func isReleaseVersion(v string) bool {
	return strings.HasPrefix(v, "v") && !pseudoVersionTailRe.MatchString(v)
}

// shortSHA truncates a full git sha to the width git prints by default.
func shortSHA(rev string) string {
	if len(rev) <= shortSHALen {
		return rev
	}
	return rev[:shortSHALen]
}

// PrettyJSON renders i as the 2-space-indented JSON object served on /version
// and printed by `version -o json`, with a trailing newline.
func (i Info) PrettyJSON() string {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		// Unreachable: every field is a string.
		return "{}\n"
	}
	return string(b) + "\n"
}
