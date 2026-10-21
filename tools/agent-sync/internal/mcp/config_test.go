package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

func parseMCP(t *testing.T, yaml string) (Config, error) {
	t.Helper()
	f, err := config.Parse("/cfg/config.yaml", []byte(yaml))
	if err != nil {
		t.Fatalf("config.Parse: %v", err)
	}
	return ParseConfig(f.MCP)
}

func TestParseConfigResolvesPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := []struct {
		name, yaml string
		want       Config
	}{
		{
			name: "defaults are the script's paths",
			yaml: "mcp:\n  servers: ~/data/servers.json\n",
			want: Config{
				Servers:      filepath.Join(home, "data/servers.json"),
				Dropins:      filepath.Join(home, ".config/claude-mcp/servers.d"),
				ProfilesFile: filepath.Join(home, ".config/ralph/config.local.toml"),
				Stamp:        filepath.Join(home, ".cache/claude-mcp/sync.hash"),
			},
		},
		{
			name: "every key set",
			yaml: `mcp:
  servers: /d/servers.json
  dropins: /d/servers.d
  profiles_file: /r/local.toml
  stamp: ~/stamp
  prune: true
`,
			want: Config{
				Servers:      "/d/servers.json",
				Dropins:      "/d/servers.d",
				ProfilesFile: "/r/local.toml",
				Stamp:        filepath.Join(home, "stamp"),
				Prune:        true,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMCP(t, tc.yaml)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("ParseConfig =\n %+v\nwant\n %+v", got, tc.want)
			}
		})
	}
}

func TestParseConfigRejects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []struct{ name, yaml, want string }{
		{"no section", "settings: {}\n", "config /cfg/config.yaml: mcp: section missing"},
		{"no servers", "mcp:\n  prune: true\n", "config /cfg/config.yaml:2: mcp.servers: required"},
		{"empty section", "mcp:\n", "mcp.servers: required"},
		{"unknown key", "mcp:\n  servers: /s.json\n  scope: user\n", ":3: mcp.scope: unknown key"},
		{
			"relative path",
			"mcp:\n  servers: data/s.json\n",
			"mcp.servers: want an absolute or ~/ path",
		},
		{"number path", "mcp:\n  servers: 7\n", "mcp.servers: want a string, got int 7"},
		{
			"prune string",
			"mcp:\n  servers: /s.json\n  prune: \"yes\"\n",
			`:3: mcp.prune: want true or false, got the string "yes"`,
		},
		{
			"prune null",
			"mcp:\n  servers: /s.json\n  prune:\n",
			"mcp.prune: want true or false, got null",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMCP(t, tc.yaml)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ParseConfig error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestDoctorChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir()) // no claude
	servers := filepath.Join(home, "servers.json")
	if err := os.WriteFile(servers, []byte(`{"servers": {"a": {"command": "/a"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, yaml string
		want       map[string]string // check name to error text; "" passes, "skip" skips
	}{
		{"no section", "settings: {}\n", map[string]string{"mcp": "skip"}},
		{
			"bad config",
			"mcp:\n  stamp: rel\n",
			map[string]string{"mcp-config": "mcp.servers: required"},
		},
		{
			"servers load, claude missing",
			"mcp:\n  servers: " + servers + "\n",
			map[string]string{
				"mcp-config":  "",
				"mcp-servers": "",
				"mcp-claude":  "`claude` CLI not found on PATH",
			},
		},
		{
			"servers missing",
			"mcp:\n  servers: " + filepath.Join(home, "absent.json") + "\n",
			map[string]string{
				"mcp-config":  "",
				"mcp-servers": "absent.json not found",
				"mcp-claude":  "not found",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := config.Parse("/cfg/config.yaml", []byte(tc.yaml))
			if err != nil {
				t.Fatal(err)
			}
			checks := DoctorChecks(f.MCP)
			if len(checks) != len(tc.want) {
				t.Fatalf("got %d checks, want %d", len(checks), len(tc.want))
			}
			for _, c := range checks {
				want, ok := tc.want[c.Name]
				if !ok {
					t.Errorf("unexpected check %q", c.Name)
					continue
				}
				checkResult(t, c, want)
			}
		})
	}
}

// checkResult runs c and compares its outcome with want: "skip", "" for a
// pass, or text the failure must contain.
func checkResult(t *testing.T, c doctor.Check, want string) {
	t.Helper()
	got := doctor.Collect(context.Background(), []doctor.Check{c}).Checks[0]
	switch want {
	case "skip":
		if got.Status != doctor.StatusSkipped {
			t.Errorf("%s = %+v, want a skip", c.Name, got)
		}
	case "":
		if got.Status != doctor.StatusOK {
			t.Errorf("%s = %+v, want ok", c.Name, got)
		}
	default:
		if got.Status != doctor.StatusFail || !strings.Contains(got.Detail, want) {
			t.Errorf("%s = %+v, want a failure containing %q", c.Name, got, want)
		}
	}
}
