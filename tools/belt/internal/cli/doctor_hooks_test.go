package cli

import (
	"fmt"
	"strings"
	"testing"
)

// readHookSettings is a Claude settings file with one Bash deny pattern,
// so the warnings section stays empty, and a PostToolUse block running
// belt hint read under matcher.
func readHookSettings(matcher string) string {
	return fmt.Sprintf(`{
  "permissions": {"deny": ["Bash(kubectl delete:*)"]},
  "hooks": {
    "PostToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "~/code/bin/belt hint bash"}]},
      {"matcher": %q, "hooks": [{"type": "command", "command": "~/code/bin/belt hint read"}]}
    ]
  }
}`, matcher)
}

// TestDoctorChecksTheReadMatcher pins the hook wiring section: the old Read
// matcher warns and names the matcher to set, the new one does not, and the
// check is skipped without a belt hint read entry or with the settings read
// off. None of it touches the warnings list, so the exit status stays clean.
func TestDoctorChecksTheReadMatcher(t *testing.T) {
	const wantWarning = `  WARNING: matcher "Read" lets no csl_read through, so read-size stays silent` +
		` after a whole-file csl_read. Set it to "Read|mcp__csl__csl_read", with your csl MCP` +
		` server's registration name as the middle segment.`
	tests := []struct {
		name, settings, config string
		want                   []string
		wantWarning            bool
	}{
		{
			name:        "old matcher",
			settings:    readHookSettings("Read"),
			want:        []string{`  matcher "Read"  (`, wantWarning},
			wantWarning: true,
		},
		{
			name:     "new matcher",
			settings: readHookSettings("Read|mcp__csl__csl_read"),
			want:     []string{`  matcher "Read|mcp__csl__csl_read"  (`},
		},
		{
			name: "csl_read in an entry of its own",
			settings: `{"permissions": {"deny": ["Bash(kubectl delete:*)"]}, "hooks": {"PostToolUse": [
  {"matcher": "Read", "hooks": [{"type": "command", "command": "belt hint read"}]},
  {"matcher": "mcp__csl__csl_read", "hooks": [{"type": "command", "command": "belt hint read"}]}
]}}`,
			want: []string{`  matcher "Read"  (`, `  matcher "mcp__csl__csl_read"  (`},
		},
		{
			name:     "no belt hint read entry",
			settings: `{"permissions": {"deny": ["Bash(kubectl delete:*)"]}}`,
			want:     []string{"  (none: read-size is not wired)"},
		},
		{
			name:     "settings read switched off",
			settings: readHookSettings("Read"),
			config: "claude_settings:\n  enabled: false\n" +
				"guards:\n  script-deny-list:\n    extra_patterns: [rm -rf]\n",
			want: []string{"  not checked (claude_settings.enabled: false)"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := armedConfig(t, dir)
			writeFile(t, dir, "settings.json", tt.settings)
			if tt.config != "" {
				writeFile(t, dir, "config.yaml", tt.config+
					"internal_names:\n  blocked_words:\n    - acmecorp\n")
			}

			out, warns := runDoctorReport(t, p)

			if !strings.Contains(
				out,
				"\nhook wiring, the PostToolUse entries in the Claude settings"+
					" that run belt hint read:\n",
			) {
				t.Errorf("hook wiring section missing:\n%s", out)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("report does not contain %q:\n%s", want, out)
				}
			}
			if got := strings.Contains(out, "WARNING: matcher"); got != tt.wantWarning {
				t.Errorf("WARNING line present = %v, want %v:\n%s", got, tt.wantWarning, out)
			}
			if len(warns) != 0 {
				t.Errorf(
					"warnings = %v, want none: the read matcher never sets the exit status",
					warns,
				)
			}
		})
	}
}

func TestMatcherSeesCSLRead(t *testing.T) {
	tests := []struct {
		matcher string
		want    bool
	}{
		{"Read", false},
		{"Read|Grep", false},
		{"Read|mcp__csl__csl_read", true},
		{"mcp__csl__csl_read|Read", true},
		{"Read|mcp__code_search__csl_read", true},
		{"Read|mcp__code-search__csl_read", true},
		{"Read|mcp__[^_]+__csl_read", true},
		{"Read|mcp__.*__csl_read", true},
		{"mcp__csl__.*", true},
		{"mcp__code-search__.*", true},
		{"Read|mcp__csl__csl_search", false},
		{"Read|csl_read", false},
		{"", true},
		{"*", true},
		{"Read|mcp__csl__(", false},
	}
	for _, tt := range tests {
		if got := matcherSeesCSLRead(tt.matcher); got != tt.want {
			t.Errorf("matcherSeesCSLRead(%q) = %v, want %v", tt.matcher, got, tt.want)
		}
	}
}

func TestRunsBeltHintRead(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{"belt hint read", true},
		{"~/code/bin/belt hint read", true},
		{"/usr/local/bin/belt hint read 2>/dev/null", true},
		{"env BELT_CONFIG=/x belt hint read", true},
		{"belt hint bash", false},
		{"belt hook read", false},
		{"suspenders hint read", false},
		{"belt hint", false},
		{"", false},
		{"belt hint session-start; belt hint read", true},
		{"belt hint session-start && ~/code/bin/belt hint read", true},
	}
	for _, tt := range tests {
		if got := runsBeltHintRead(tt.command); got != tt.want {
			t.Errorf("runsBeltHintRead(%q) = %v, want %v", tt.command, got, tt.want)
		}
	}
}

// TestBeltHint: a hook command is read the way the belt binary parses its
// arguments, so a flag before the event, a global flag before hint, and a
// full path to the binary all count, while words after a shell operator
// belong to another command. Each case asks for one event, and a chain of
// commands is searched for that event past a hint for another one.
func TestBeltHint(t *testing.T) {
	tests := []struct {
		name, command string
		wantOK        bool
		event, agent  string // event is the one asked for
	}{
		{"plain", "belt hint session-start", true, "session-start", ""},
		{
			"flag after the event",
			"belt hint session-start --agent claude-code",
			true,
			"session-start",
			"claude-code",
		},
		{
			"flag as one word",
			"belt hint session-start --agent=claude-code",
			true,
			"session-start",
			"claude-code",
		},
		{
			"flag before the event",
			"belt hint --agent claude-code session-start",
			true,
			"session-start",
			"claude-code",
		},
		{
			"one-word flag before the event",
			"belt hint --agent=claude-code session-start",
			true,
			"session-start",
			"claude-code",
		},
		{
			"global flag before hint",
			"belt --config /x/config.yaml hint session-start --agent claude-code",
			true,
			"session-start",
			"claude-code",
		},
		{
			"one-word global flag",
			"belt --config=/x/config.yaml hint --agent claude-code session-start",
			true,
			"session-start",
			"claude-code",
		},
		{
			"full path",
			"/Users/me/code/bin/belt hint session-start --agent claude-code",
			true,
			"session-start",
			"claude-code",
		},
		{
			"quoted path",
			`"$HOME/code/bin/belt" hint session-start --agent claude-code`,
			true,
			"session-start",
			"claude-code",
		},
		{
			"agent and event lowercased",
			"belt hint Session-Start --agent Claude-Code",
			true,
			"session-start",
			"claude-code",
		},
		{"env prefix", "env BELT_CONFIG=/x belt hint read", true, "read", ""},
		{
			"second command",
			"belt watch && belt hint session-start --agent claude-code",
			true,
			"session-start",
			"claude-code",
		},
		{
			"flag after an operator",
			"belt hint session-start && echo --agent claude-code",
			true,
			"session-start",
			"",
		},
		{
			"flag after a semicolon",
			"belt hint session-start; echo --agent claude-code",
			true,
			"session-start",
			"",
		},
		{
			"double-quoted flag value",
			`belt hint session-start --agent="claude-code"`,
			true,
			"session-start",
			"claude-code",
		},
		{
			"single-quoted flag value",
			`belt hint --agent='claude-code' session-start`,
			true,
			"session-start",
			"claude-code",
		},
		{
			"asked event after another hint",
			"belt hint session-start; belt hint read",
			true,
			"read",
			"",
		},
		{
			"asked event after another hint, no flag",
			"belt hint read && belt hint session-start",
			true,
			"session-start",
			"",
		},
		{
			"first hint for the asked event wins",
			"belt hint read && belt hint session-start --agent claude-code && belt hint session-start",
			true,
			"session-start",
			"claude-code",
		},
		{"another event", "belt hint read", false, "session-start", ""},
		{
			"global flag eats the next word",
			"belt --config hint session-start",
			false,
			"session-start",
			"",
		},
		{"no event", "belt hint", false, "read", ""},
		{"another command", "belt hook read", false, "read", ""},
		{"another binary", "suspenders hint read", false, "read", ""},
		{"binary named like belt", "belt-wrapper hint read", false, "read", ""},
		{"empty", "", false, "read", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, ok := beltHint(tt.command, tt.event)
			if ok != tt.wantOK || agent != tt.agent {
				t.Errorf("beltHint(%q, %q) = %q, %v; want %q, %v",
					tt.command, tt.event, agent, ok, tt.agent, tt.wantOK)
			}
		})
	}
}
