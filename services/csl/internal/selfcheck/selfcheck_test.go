package selfcheck

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

func TestCheckNamesAndOrder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CSL_CONFIG", "")
	want := []string{
		"config-loads",
		"state-file-loads",
		"index-freshness",
		"index-shards-valid",
		"search-server-responsive",
		"web-ui-reachable",
		"web-ui-version-skew",
	}
	checks := Checks(false)
	if len(checks) != len(want) {
		t.Fatalf("Checks returned %d checks, want %d", len(checks), len(want))
	}
	for i, c := range checks {
		if c.Name != want[i] {
			t.Errorf("check %d name = %q, want %q", i, c.Name, want[i])
		}
	}
}

// TestConfigLoads covers the "why is the UI empty" case: a config that parses
// but configures nothing to walk has to fail, naming the file to edit.
func TestConfigLoads(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{name: "dirs set passes", yaml: "dirs:\n  - /tmp\n"},
		{name: "empty dirs fails", yaml: "index:\n  hosts: []\n", wantErr: "sets no dirs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, path, tc.yaml)
			config.SetPath(path)
			t.Cleanup(func() { config.SetPath("") })

			err := configLoads().Run(context.Background())
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Run() = %v, want nil", err)
			case tc.wantErr == "":
			case err == nil:
				t.Fatalf("Run() = nil, want an error mentioning %q", tc.wantErr)
			case !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			case !strings.Contains(err.Error(), path):
				t.Errorf("error %q does not name the config file %q", err, path)
			}
		})
	}
}

func TestStateFileLoads_CorruptWithoutRepair(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "state.json"), "{not json")

	err := stateFileLoads(dir, false).Run(context.Background())
	if err == nil {
		t.Fatal("stateFileLoads on corrupt state = nil, want error")
	}
	if !strings.Contains(err.Error(), "--repair") {
		t.Errorf("error %q does not mention --repair", err)
	}
}

func TestStateFileLoads_RepairResets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "state.json"), "{not json")

	if err := stateFileLoads(dir, true).Run(context.Background()); err != nil {
		t.Fatalf("stateFileLoads with repair = %v, want nil", err)
	}
	if _, err := search.LoadState(dir); err != nil {
		t.Errorf("state after repair does not load: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json.corrupt")); err != nil {
		t.Errorf("corrupt backup missing: %v", err)
	}
}

func TestIndexShardsValid(t *testing.T) {
	t.Run("empty dir passes", func(t *testing.T) {
		if err := indexShardsValid(t.TempDir()).Run(context.Background()); err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	})

	t.Run("corrupt shard fails with repair hint", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "bad.zoekt"), "not a shard")
		err := indexShardsValid(dir).Run(context.Background())
		if err == nil {
			t.Fatal("Run() = nil, want error for corrupt shard")
		}
		if !strings.Contains(err.Error(), "csl index --repair") {
			t.Errorf("error %q does not name the repair command", err)
		}
	})
}

func TestSearchServerResponsive(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")
	socketPath := filepath.Join(dir, "daemon.sock")

	t.Run("not running passes", func(t *testing.T) {
		if err := searchServerResponsive(pidPath, socketPath).Run(context.Background()); err != nil {
			t.Errorf("Run() = %v, want nil", err)
		}
	})

	t.Run("alive pid with dead socket fails", func(t *testing.T) {
		writeFile(t, pidPath, strconv.Itoa(os.Getpid()))
		err := searchServerResponsive(pidPath, socketPath).Run(context.Background())
		if err == nil {
			t.Fatal("Run() = nil, want error for live pid with unresponsive socket")
		}
	})
}

// TestNoConfigPasses is the zero-config contract on the doctor side: a
// machine with no config file has nothing configured to index, which is a
// starting state rather than a fault, so the config and freshness checks pass
// and the path to create arrives as a note instead.
func TestNoConfigPasses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CSL_CONFIG", "")
	config.SetPath("")

	// The check skips rather than passing silently or failing: a report an
	// agent reads has to say why there is nothing to index.
	report := doctor.Collect(context.Background(), []doctor.Check{configLoads()})
	if !report.OK {
		t.Errorf("Collect().OK = false, want a skip to count as a pass")
	}
	if got := report.Checks[0].Status; got != doctor.StatusSkipped {
		t.Errorf("config-loads status = %q, want %q", got, doctor.StatusSkipped)
	}
	wantPath := filepath.Join(home, ".config", "csl", "config.yaml")
	if got := report.Checks[0].Detail; !strings.Contains(got, wantPath) {
		t.Errorf("config-loads detail = %q, want it to name %q", got, wantPath)
	}
	if err := indexFreshness(filepath.Join(home, "index")).Run(context.Background()); err != nil {
		t.Errorf("index-freshness with no config file = %v, want nil", err)
	}
}

// TestWebBaseURLFollowsPort pins the fix for links that outlived a port
// change: with no web.base_url configured, the probes follow CSL_PORT.
func TestWebBaseURLFollowsPort(t *testing.T) {
	config.SetPath(filepath.Join(t.TempDir(), "absent.yaml"))
	t.Cleanup(func() { config.SetPath("") })
	t.Setenv("CSL_PORT", "9424")

	if got, want := webBaseURL(), "http://127.0.0.1:9424"; got != want {
		t.Errorf("webBaseURL() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
