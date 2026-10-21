package mcp

import (
	"maps"
	"slices"
	"testing"
)

// TestParseGetReadsWhatAddWrote covers the get output the case fixtures do
// not: an Environment block whose values hold "," and "=", one that ends
// at the next line of its own indent, a Headers block whose header names
// match get's labels, and the bare Args line of a server with no args.
func TestParseGetReadsWhatAddWrote(t *testing.T) {
	cases := []struct {
		name, stdout string
		want         registered
	}{
		{
			name: "block values keep commas and equals signs",
			stdout: "alpha:\n  Scope: User config (available in all your projects)\n" +
				"  Type: stdio\n  Command: /opt/alpha\n  Args: --name two words\n" +
				"  Environment:\n    QUERY=a=1,b=2\n    LIST=x, y\n\nTo remove this server, run: ...\n",
			want: registered{
				scope: "user", transport: transportStdio, command: "/opt/alpha",
				args: "--name two words",
				env:  map[string]string{"QUERY": "a=1,b=2", "LIST": "x, y"},
			},
		},
		{
			name: "a block ends at a line of its own indent",
			stdout: "bravo:\n  Scope: Local config (private to you in this project)\n" +
				"  Environment:\n    PORT=7001\n  Command: /opt/bravo\n",
			want: registered{
				scope: "local", transport: transportStdio, command: "/opt/bravo",
				env: map[string]string{"PORT": "7001"},
			},
		},
		{
			name: "a Headers block is not read as labels",
			stdout: "charlie:\n  Scope: User config (available in all your projects)\n" +
				"  Type: sse\n  URL: https://mcp.example.com/sse\n  Headers:\n" +
				"    URL: https://other.example.com\n    Type: stdio\n    Command: /bin/other\n" +
				"    Scope: Local\n    Environment: K=v\n\nTo remove this server, run: ...\n",
			want: registered{
				scope: "user", transport: "sse", url: "https://mcp.example.com/sse",
				env: map[string]string{},
			},
		},
		{
			name: "bare Args and Environment lines are empty blocks (claude 2.1.306)",
			stdout: "plain:\n  Scope: User config (available in all your projects)\n" +
				"  Type: stdio\n  Command: /usr/bin/true\n  Args:\n  Environment:\n\n" +
				"To remove this server, run: claude mcp remove plain -s user\n",
			want: registered{
				scope: "user", transport: transportStdio, command: "/usr/bin/true",
				env: map[string]string{},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseGet(c.stdout)
			if !ok {
				t.Fatal("parseGet: ok = false, want true")
			}
			if got.scope != c.want.scope || got.transport != c.want.transport ||
				got.command != c.want.command || got.url != c.want.url ||
				got.args != c.want.args || !maps.Equal(got.env, c.want.env) {
				t.Errorf("parseGet = %+v, want %+v", got, c.want)
			}
		})
	}
}

// listDiagnostics is the footer claude 2.1.306 prints below the entries of
// a piped `claude mcp list` when a name is defined at two scopes, with its
// OSC 8 hyperlink.
const listDiagnostics = "\nMCP config diagnostics \u26a0\n\n" +
	"For help configuring MCP servers, see: " +
	"\x1b]8;id=c415zw;https://code.claude.com/docs/en/mcp\x1b\\" +
	"https://code.claude.com/docs/en/mcp\x1b]8;;\x1b\\\n\n" +
	"[Conflicting scopes]\n" +
	"\u251c Server \"shadow\" is defined in multiple scopes with different endpoints: " +
	"user (/bin/echo user), local (/bin/echo local). OAuth tokens are stored per endpoint, " +
	"so authenticating in one context will not carry over.\n" +
	"\u2514 Keep the correct endpoint and remove the others: " +
	"`claude mcp remove shadow -s user` or `claude mcp remove shadow -s local`\n\n"

// TestParseListReadsOnlyEntries checks that list's banner, no-servers line
// and diagnostics footer never read as server names, and that a name add
// would reject is set apart as invalid unless it is a connector's or a
// plugin's.
func TestParseListReadsOnlyEntries(t *testing.T) {
	cases := []struct {
		name, stdout string
		want         []string
		wantInvalid  []string
	}{
		{
			name: "no servers",
			stdout: "No MCP servers configured. " +
				"Use `claude mcp add` to add a server.\n",
		},
		{
			name: "the diagnostics footer below the entries",
			stdout: "Checking MCP server health\u2026\n\n" +
				"alpha: /opt/alpha/bin/alpha  - Connected\n" +
				"shadow: /bin/echo local - Connected\n" + listDiagnostics,
			want: []string{"alpha", "shadow"},
		},
		{
			name:   "a footer with no entries above it",
			stdout: "Checking MCP server health\u2026\n" + listDiagnostics,
		},
		{
			name: "connector, plugin and rejected names",
			stdout: "Checking MCP server health...\n\n" +
				"under_score-1: /opt/u - Connected\n" +
				"claude.ai Calendar: https://calendar.example.com/mcp - Connected\n" +
				"plugin:tools:lint: /opt/lint/bin/lint - Connected\n" +
				"dotted.name: /opt/d - Connected\n" +
				"two words: /opt/t - Connected\n" +
				"bravo:/opt/bravo/bin/bravo\n" +
				"a line without a colon\n",
			want: []string{
				"bravo", "claude.ai Calendar", "plugin:tools:lint", "under_score-1",
			},
			wantInvalid: []string{"dotted.name", "two words"},
		},
		{
			name: "a name that starts like the banner",
			stdout: "Checking MCP server health\u2026\n\n" +
				"Checking-svc: /opt/c - Connected\n",
			want: []string{"Checking-svc"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := parseList(c.stdout)
			got := slices.Sorted(maps.Keys(l.names))
			if !slices.Equal(got, c.want) || !slices.Equal(l.invalid, c.wantInvalid) {
				t.Errorf("parseList = %q, invalid %q; want %q, invalid %q",
					got, l.invalid, c.want, c.wantInvalid)
			}
		})
	}
}
