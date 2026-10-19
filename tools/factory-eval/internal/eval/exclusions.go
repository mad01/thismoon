package eval

import "strings"

// userExclusions sorts the user's sandbox.excludedCommands into the
// entries a run denies and the entries that stay unsandboxed. Exclusion
// lists combine across the settings files a session loads, so the run's
// settings file cannot take an entry back; what it can do is deny the
// same pattern, and a deny rule beats every allow rule, so the command
// never runs at all. An entry whose command the arms' own allow list
// needs, git on this machine, cannot be denied that way: it runs outside
// the sandbox, gated by the permission rules and the fingerprint only,
// and the results name it under unsandboxed_commands.
func userExclusions(excluded, allow []string) (deny, unsandboxed []string) {
	needed := make(map[string]bool, len(allow))
	for _, rule := range allow {
		if body, ok := strings.CutPrefix(rule, "Bash("); ok {
			needed[commandWord(strings.TrimSuffix(body, ")"))] = true
		}
	}
	for _, entry := range excluded {
		if needed[commandWord(entry)] {
			unsandboxed = append(unsandboxed, entry)
			continue
		}
		deny = append(deny, "Bash("+entry+")")
	}
	return deny, unsandboxed
}

// commandWord is the command an exclusion entry or a Bash rule body
// names: the text before the first space or colon, so `git:*`, `git *`
// and `git -C * add *` all give git.
func commandWord(pattern string) string {
	end := strings.IndexAny(pattern, " :")
	if end < 0 {
		return pattern
	}
	return pattern[:end]
}
