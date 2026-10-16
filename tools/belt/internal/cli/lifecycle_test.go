package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestConfigRendersResolvedLifecycle: with no config file, the effective
// view still spells out every ledger and watch value belt runs with.
func TestConfigRendersResolvedLifecycle(t *testing.T) {
	out := runConfigDocString(t, doctorPaths(t.TempDir()))
	_, body, _ := strings.Cut(out, "\n\n")
	var got effectiveConfig
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if got.Ledger.Enabled == nil || !*got.Ledger.Enabled || got.Ledger.Source != "belt-ledger" {
		t.Errorf("ledger = %+v, want enabled under belt-ledger", got.Ledger)
	}
	want := config.Watch{PollIntervalMS: 2000, MaxWaitS: 540, Source: "belt-inbox"}
	if got.Watch != want {
		t.Errorf("watch = %+v, want %+v", got.Watch, want)
	}
}

// TestConfigRendersClampedLifecycleValues: `belt config` prints the values
// belt runs with, so a clamped or normalized value shows in its effective
// form rather than as written.
func TestConfigRendersClampedLifecycleValues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "config.yaml",
		"ledger:\n  source: Belt_Ledger\nwatch:\n  poll_interval_ms: 50\n  max_wait_s: 86400\n")
	out := runConfigDocString(t, doctorPaths(dir))
	_, body, _ := strings.Cut(out, "\n\n")
	var got effectiveConfig
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if got.Ledger.Source != "belt_ledger" {
		t.Errorf("ledger source = %q, want the normalized belt_ledger", got.Ledger.Source)
	}
	want := config.Watch{PollIntervalMS: 100, MaxWaitS: 3600, Source: "belt-inbox"}
	if got.Watch != want {
		t.Errorf("watch = %+v, want %+v", got.Watch, want)
	}
}

// TestBadLifecycleValuesNeverDenyAToolCall pins the reason those values are
// clamped rather than rejected: a rejected value is a config error, and a
// config error makes every guarded tool call deny. Each config below once
// made `belt hook bash` deny `ls`; none may now, watch on or off.
func TestBadLifecycleValuesNeverDenyAToolCall(t *testing.T) {
	for _, cfgText := range []string{
		"watch:\n  enabled: false\n  poll_interval_ms: 50\n",
		"ledger:\n  enabled: false\n  source: Belt_Ledger\n",
		"watch:\n  enabled: true\n  poll_interval_ms: -1\n  max_wait_s: -5\n  source: 'in box/1'\n",
		"watch:\n  max_wait_s: 999999\n",
		"ledger:\n  source: '///'\n",
	} {
		t.Run(strings.ReplaceAll(cfgText, "\n", " "), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			cfgPath := writeFile(t, dir, "config.yaml", cfgText)
			root := rootCmd()
			root.SetArgs([]string{"--config", cfgPath, "hook", "bash"})
			root.SetIn(strings.NewReader(
				`{"tool_name":"Bash","tool_input":{"command":"ls"},"cwd":"` + dir + `"}`,
			))
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)

			if err := root.Execute(); err != nil {
				t.Fatalf("hook bash error = %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("hook bash wrote %q for ls, want no decision", out.String())
			}
		})
	}
}

// TestConfigReferenceIsAValidConfig keeps the annotated reference in `belt
// config --help` honest: pasted into a config file as is, it must load and
// validate, so it can never show a key or value validation rejects.
func TestConfigReferenceIsAValidConfig(t *testing.T) {
	dir := t.TempDir()
	p := config.Paths{BeltYAML: writeFile(t, dir, "config.yaml", configReference)}
	cfg, err := config.LoadFrom(p)
	if err != nil {
		t.Fatalf("the reference does not load: %v", err)
	}
	if cfg.CustomHints["worker-brief"].Event != config.HintEventSubagentStart {
		t.Errorf("reference lost its subagent-start example: %+v", cfg.CustomHints)
	}
	if !cfg.Ledger.On() || cfg.Watch.Enabled ||
		cfg.Watch.SourceName() != config.DefaultWatchSource {
		t.Errorf(
			"reference lifecycle = %+v / %+v, want the defaults spelled out",
			cfg.Ledger,
			cfg.Watch,
		)
	}
}

// TestWatchCmdExitContract pins how `belt watch` reaches the wake exit
// code: a delivery returns errWake (Execute turns it into exit 2) with the
// items on stderr, and a disabled watcher returns nil with nothing written.
func TestWatchCmdExitContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id": "0001", "title": "your turn",
			"tags": map[string]string{"to": "s1", "from": "orchestrator"},
		}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("EVENTS_BASE_URL", srv.URL)

	tests := []struct {
		name       string
		config     string
		wantErr    error
		wantStderr string
	}{
		{
			"enabled watcher delivers", "watch:\n  enabled: true\n  max_wait_s: 1\n", errWake,
			"belt[watch]: from orchestrator: your turn\n",
		},
		{"disabled watcher stays silent", "hints: {}\n", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			cfgPath := writeFile(t, dir, "config.yaml", tt.config)
			root := rootCmd()
			root.SetArgs([]string{"--config", cfgPath, "watch"})
			root.SetIn(strings.NewReader(`{"session_id":"s1","hook_event_name":"Stop"}`))
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)

			err := root.Execute()

			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Errorf("Execute() = %v, want %v", err, tt.wantErr)
			}
			if errOut.String() != tt.wantStderr || out.Len() != 0 {
				t.Errorf(
					"stdout %q, stderr %q, want stderr %q only",
					out.String(),
					errOut.String(),
					tt.wantStderr,
				)
			}
		})
	}
}
