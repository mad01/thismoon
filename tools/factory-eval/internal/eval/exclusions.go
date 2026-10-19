package eval

import "strings"

// wrappers are the command words Claude Code strips before it matches a
// Bash permission rule, so `timeout 60 go test ./...` matches a go test
// rule. The exclusion docs do not say the same of exclusion matching, so
// an entry naming one may take a wrapped call out of the sandbox while a
// deny rule for the entry never fires. Such an entry counts as unsandboxed.
var wrappers = map[string]bool{
	"timeout": true, "time": true, "nice": true, "nohup": true,
	"stdbuf": true, "command": true, "builtin": true, "noglob": true,
}

// userExclusions sorts the user's sandbox.excludedCommands into the
// entries a run denies and the entries that stay unsandboxed. Exclusion
// lists combine across the settings files a session loads, so the run's
// settings file cannot take an entry back; what it can do is deny the
// same pattern, and a deny rule beats every allow rule, so the command
// never runs at all. An entry that overlaps an arm allow rule, git here,
// cannot be denied that way: the command runs outside the sandbox, gated
// by the permission rules and the fingerprint only, and the results name
// it under unsandboxed_commands. An entry naming a wrapper word stays
// unsandboxed for the reason wrappers gives.
func userExclusions(excluded, allow []string) (deny, unsandboxed []string) {
	var prefixes []string
	for _, rule := range allow {
		if body, ok := strings.CutPrefix(rule, "Bash("); ok {
			prefixes = append(prefixes, literalPrefix(strings.TrimSuffix(body, ")")))
		}
	}
	for _, entry := range excluded {
		p := literalPrefix(entry)
		if wrappers[commandWord(p)] || overlapsAny(p, prefixes) {
			unsandboxed = append(unsandboxed, entry)
			continue
		}
		deny = append(deny, "Bash("+entry+")")
	}
	return deny, unsandboxed
}

// Unsandboxed is the user's exclusions a run leaves unsandboxed, for the
// gate that refuses a paid batch over them.
func Unsandboxed(excluded []string) []string {
	_, unsandboxed := userExclusions(excluded, bashTools())
	return unsandboxed
}

// literalPrefix is the text of a rule body or an exclusion entry before
// its first wildcard, with the colon of a `cmd:*` suffix dropped: `git:*`
// gives git, `git *` gives "git ", `git -C * add *` gives "git -C ", a
// bare `docker` gives itself and `*` gives the empty string.
func literalPrefix(pattern string) string {
	if i := strings.Index(pattern, "*"); i >= 0 {
		pattern = pattern[:i]
	}
	return strings.TrimSuffix(pattern, ":")
}

// overlapsAny reports whether p and one of the prefixes cover a common
// command: either is a prefix of the other. "git" overlaps "git add ",
// "git fetch " overlaps neither "git add " nor "git -C ".
func overlapsAny(p string, prefixes []string) bool {
	for _, q := range prefixes {
		if strings.HasPrefix(q, p) || strings.HasPrefix(p, q) {
			return true
		}
	}
	return false
}

// commandWord is the first word of a literal prefix.
func commandWord(prefix string) string {
	if end := strings.IndexByte(prefix, ' '); end >= 0 {
		return prefix[:end]
	}
	return prefix
}
