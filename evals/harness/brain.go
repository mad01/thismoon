package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
)

// BrainCall is one brain invocation found in a Bash call's command.
// Subcommand is the first argument after brain ("search", "--help"), or ""
// for a bare brain. Help is set when a later argument asks for help.
type BrainCall struct {
	Subcommand string `json:"subcommand"`
	Help       bool   `json:"help,omitempty"`
	Command    string `json:"command"`
}

// noSubcommand labels a bare `brain` call in counts.
const noSubcommand = "(none)"

// SubcommandLabel names the call in counts and brain_cmd matching:
// "search", "write --help" for a help lookup, "--help", or noSubcommand.
func (b BrainCall) SubcommandLabel() string {
	switch {
	case b.Subcommand == "":
		return noSubcommand
	case b.Help:
		return b.Subcommand + " --help"
	}
	return b.Subcommand
}

// isHelp reports whether the call only looks up help: `brain --help`,
// `brain help` or `brain <command> --help`.
func (b BrainCall) isHelp() bool {
	return b.Help || isHelpFlag(b.Subcommand)
}

// brainCallsIn returns every brain invocation in the Bash calls among
// calls that were allowed to run, in order. One Bash call can hold several.
func brainCallsIn(calls []ToolCall) []BrainCall {
	var out []BrainCall
	for _, c := range calls {
		if c.Name == "Bash" && !c.Denied {
			out = append(out, brainCalls(bashCommand(c.Input))...)
		}
	}
	return out
}

// brainCommandCounts counts brain calls by SubcommandLabel, or returns nil
// when there are none.
func brainCommandCounts(calls []BrainCall) map[string]int {
	if len(calls) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, c := range calls {
		counts[c.SubcommandLabel()]++
	}
	return counts
}

// bashCommand returns the command of a Bash call's input, or "".
func bashCommand(input string) string {
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return ""
	}
	return in.Command
}

// commandPrefixes are words that run the command after them, so
// `time brain search x` and `xargs brain read` still count as brain calls.
var commandPrefixes = map[string]bool{
	"do": true, "then": true, "else": true, "time": true,
	"exec": true, "xargs": true, "command": true, "!": true,
}

// brainCalls finds the brain invocations in a shell command. It splits the
// command into simple commands on unquoted ; & | ( ) ` and newlines, skips
// leading VAR=value assignments and command prefixes, and keeps the
// segments whose program is brain. Heredoc bodies are not recognised, so a
// heredoc line that starts with brain counts as a call.
func brainCalls(command string) []BrainCall {
	var out []BrainCall
	for _, seg := range splitSimpleCommands(command) {
		fields := strings.Fields(seg)
		for len(fields) > 0 && (isAssignment(fields[0]) || commandPrefixes[fields[0]]) {
			fields = fields[1:]
		}
		if len(fields) == 0 || filepath.Base(fields[0]) != "brain" {
			continue
		}
		call := BrainCall{Command: strings.TrimSpace(seg)}
		if len(fields) > 1 {
			call.Subcommand = strings.Trim(fields[1], `"'`)
			call.Help = slices.ContainsFunc(fields[2:], isHelpFlag)
		}
		out = append(out, call)
	}
	return out
}

// splitSimpleCommands splits command on shell separators outside quotes.
func splitSimpleCommands(command string) []string {
	var (
		out    []string
		cur    strings.Builder
		quote  rune
		escape bool
	)
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			out = append(out, s)
		}
		cur.Reset()
	}
	for _, r := range command {
		switch {
		case escape:
			escape = false
		case r == '\\' && quote != '\'':
			escape = true
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case strings.ContainsRune(";&|()`\n", r):
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

func isHelpFlag(word string) bool {
	return word == "--help" || word == "-h" || word == "help"
}

// isAssignment reports whether word is a shell VAR=value assignment.
func isAssignment(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		letter := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		digit := i > 0 && r >= '0' && r <= '9'
		if !letter && !digit {
			return false
		}
	}
	return true
}
