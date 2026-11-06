package store

import "strings"

// The harnesses moondb knows: the agent runtime a session ran in.
// HarnessClaudeCode is the default, since moondb's hooks and importer
// started with Claude Code; the pi extension (services/moondb/pi) records
// pi sessions through the same hooks and says so.
const (
	HarnessClaudeCode = "claude-code"
	HarnessPi         = "pi"
)

// piEntrypointPrefix starts the entrypoint the pi extension sets
// (pi-tui, pi-print, pi-json, pi-rpc).
const piEntrypointPrefix = "pi-"

// ParseHarness is raw when it names a known harness, else empty.
func ParseHarness(raw string) string {
	switch raw {
	case HarnessClaudeCode, HarnessPi:
		return raw
	}
	return ""
}

// SettleHarness is the harness a session keeps when an envelope says
// next: a known harness fills an empty one, and replaces the default
// claude-code, which every envelope carried before a harness could say
// otherwise. Any other harness keeps its first value.
func SettleHarness(cur, next string) string {
	if next == "" || cur == next {
		return cur
	}
	if cur == "" || cur == HarnessClaudeCode {
		return next
	}
	return cur
}

// SessionHarness is s's harness, reading a session the pi extension
// recorded before it could name its harness as pi: such a session carries
// claude-code and a pi-<mode> entrypoint.
func SessionHarness(s Session) string {
	if s.Harness == HarnessClaudeCode && strings.HasPrefix(s.Entrypoint, piEntrypointPrefix) {
		return HarnessPi
	}
	return s.Harness
}
