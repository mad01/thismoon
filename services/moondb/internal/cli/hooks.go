package cli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/moondb"
)

// recordHooks are the hook entries capture needs, named as the hooks-wired
// check reports them: the event, and the tool a PostToolUse matcher must
// select ("" for an event without tools).
var recordHooks = []struct{ event, tool string }{
	{"UserPromptSubmit", ""},
	{"PostToolUse", "Edit"},
	{"PostToolUse", "Bash"},
	{"Stop", ""},
	{"SubagentStop", ""},
}

// claudeSettings is the hooks part of a Claude Code settings file.
type claudeSettings struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// hooksWired is the hooks-wired check over the configured settings file.
func hooksWired(context.Context) error {
	return checkHooksWired(
		cmp.Or(cfg.ClaudeSettings, moondb.DefaultClaudeSettings), cfg.Capture.enabled())
}

// checkHooksWired fails, naming each one missing, unless the Claude Code
// settings file at path runs `moondb record` on every hook in recordHooks.
// A machine with capture off passes with a note.
func checkHooksWired(path string, captureOn bool) error {
	if !captureOn {
		return doctor.Skip("capture.enabled is false; hooks not checked")
	}
	path, err := confdir.Expand(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w; the moondb record hooks belong there (config.md, Hook wiring)", err)
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	var missing []string
	for _, h := range recordHooks {
		if !s.runsRecord(h.event, h.tool) {
			missing = append(missing, strings.TrimSpace(h.event+" "+h.tool))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s has no moondb record hook for %s; add them (config.md, Hook wiring)",
			path, strings.Join(missing, ", "))
	}
	return nil
}

// runsRecord reports whether a group of event whose matcher selects tool
// runs `moondb record`: a moondb binary by any path, quoted or not, record
// among its arguments.
func (s claudeSettings) runsRecord(event, tool string) bool {
	for _, g := range s.Hooks[event] {
		if tool != "" && !selects(g.Matcher, tool) {
			continue
		}
		for _, h := range g.Hooks {
			words := commandWords(h.Command)
			i := slices.IndexFunc(
				words,
				func(w string) bool { return filepath.Base(w) == "moondb" },
			)
			if i >= 0 && slices.Contains(words[i+1:], "record") {
				return true
			}
		}
	}
	return false
}

// commandWords splits a hook command on spaces and trims the single and
// double quotes around each word, so "$HOME/code/bin/moondb" reads as a
// path ending in moondb.
func commandWords(command string) []string {
	words := strings.Fields(command)
	for i, w := range words {
		words[i] = strings.Trim(w, `"'`)
	}
	return words
}

// selects reports whether a hook matcher selects tool: an empty matcher
// or * selects every tool, anything else is a pattern over the whole name.
func selects(matcher, tool string) bool {
	if matcher == "" || matcher == "*" || matcher == tool {
		return true
	}
	re, err := regexp.Compile("^(?:" + matcher + ")$")
	return err == nil && re.MatchString(tool)
}
