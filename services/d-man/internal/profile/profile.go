// Package profile switches the d-man block-list profile. A profile is a name
// whose block list, when it has one, goes above a base route set to form the
// active routes file the daemon reads.
//
// Every file lives beside the active routes file: BaseFile holds the routes
// all profiles share, BlocklistFile(name) holds one profile's blocked hosts,
// and RecordFile names the active profile. Which hosts a profile blocks is
// machine-private data, so none of it is compiled in.
//
// Render and CheckName are pure; Switch and Current are the filesystem shell
// (switch.go).
package profile

import (
	"fmt"
	"regexp"
	"slices"
)

const (
	// Default is the profile in effect when none is recorded. It is the one
	// profile that needs no block-list file: without one it blocks nothing.
	Default = "personal"
	// BaseFile is the base route set every profile starts from.
	BaseFile = "routes.base.toml"
	// RecordFile holds the name of the active profile.
	RecordFile = "profile"
)

// namePattern keeps a profile name a safe file-name segment: lowercase
// letters, digits and inner hyphens, so a name can never reach outside the
// config directory.
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// tildeGamesDir matches a games_dir value that starts with ~/, in either TOML
// string quote. Only the key's own line matches; a comment or another key
// that mentions games_dir is left alone.
var tildeGamesDir = regexp.MustCompile(`(?m)^([ \t]*games_dir[ \t]*=[ \t]*["'])~/`)

// The block-list file name is blocklistPrefix + profile + blocklistSuffix.
const (
	blocklistPrefix = "blocklist."
	blocklistSuffix = ".toml"
)

// BlocklistFile returns the block-list file name for the profile name.
func BlocklistFile(name string) string {
	return blocklistPrefix + name + blocklistSuffix
}

// CheckName rejects a profile name that is not a safe file-name segment.
func CheckName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf(
			"invalid profile name %q: use lowercase letters, digits and hyphens", name)
	}
	return nil
}

// Render returns the active routes file for one profile: blocklist above
// base, then a leading ~/ in games_dir rewritten to home. The block list goes
// first because blocklist is a top-level key, and TOML reads any key below
// the first [[route]] header as a field of that route. A nil blocklist
// renders base alone.
//
// The rewrite exists because the daemon runs as root, where ~ expands to
// /var/root; the profile command runs as the user and knows the right home.
func Render(base, blocklist []byte, home string) []byte {
	out := make([]byte, 0, len(blocklist)+1+len(base))
	out = append(out, blocklist...)
	if len(blocklist) > 0 && blocklist[len(blocklist)-1] != '\n' {
		// Without the newline the block list's last line would run into the
		// base file's first line.
		out = append(out, '\n')
	}
	out = append(out, base...)
	return tildeGamesDir.ReplaceAllFunc(out, func(match []byte) []byte {
		prefix := match[:len(match)-len("~/")]
		return append(append(slices.Clone(prefix), home...), '/')
	})
}
