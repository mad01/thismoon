package config

import (
	"iter"
	"strings"
)

// agentTypeGlob is the one wildcard an agent type pattern takes: at the end
// of a pattern it matches any rest of the name.
const agentTypeGlob = "*"

// MatchAgentType picks the pattern that names agentType, for the
// ledger.agent_types map and a custom hint's agent_types gate alike. A
// pattern ending in * matches every agent type that starts with the rest of
// it; any other pattern matches only itself, case included. A named spawn
// reports its name as its agent type, so one glob such as reviewer-* covers
// every spawn named reviewer-<slug>. The pattern equal to agentType
// wins, otherwise the longest matching glob, so the order of patterns does
// not matter. ok is false when none matches, and always for an empty
// agentType: an event without one has nothing to match.
func MatchAgentType(patterns iter.Seq[string], agentType string) (best string, ok bool) {
	if agentType == "" {
		return "", false
	}
	for p := range patterns {
		if p == agentType {
			return p, true
		}
		prefix, glob := strings.CutSuffix(p, agentTypeGlob)
		if glob && strings.HasPrefix(agentType, prefix) && (!ok || len(p) > len(best)) {
			best, ok = p, true
		}
	}
	return best, ok
}

// agentTypePatternProblem names what is wrong with an agent type pattern,
// or "" for a usable one. A * anywhere but the end looks like a glob but
// would match only its literal self, so it is malformed.
func agentTypePatternProblem(p string) string {
	if i := strings.Index(p, agentTypeGlob); i >= 0 && i < len(p)-1 {
		return "has a * before its end (only a trailing * is a glob)"
	}
	return ""
}
