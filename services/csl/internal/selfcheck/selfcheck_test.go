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
		"repos-discovered",
		"catalog-descriptors",
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

			err := configLoads(newShared()).Run(context.Background())
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
	report := doctor.Collect(context.Background(), []doctor.Check{configLoads(newShared())})
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
	if err := indexFreshness(filepath.Join(home, "index"), newShared()).Run(context.Background()); err != nil {
		t.Errorf("index-freshness with no config file = %v, want nil", err)
	}
	err := reposDiscovered(newShared()).Run(context.Background())
	if err == nil || !isSkip(err) {
		t.Fatalf("repos-discovered with no config file = %v, want a skip", err)
	}
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("repos-discovered note %q does not name %q", err, wantPath)
	}
}

// TestWebBaseURLFollowsPort pins the fix for links that outlived a port
// change: with no web.base_url configured, the probes follow CSL_PORT.
func TestWebBaseURLFollowsPort(t *testing.T) {
	config.SetPath(filepath.Join(t.TempDir(), "absent.yaml"))
	t.Cleanup(func() { config.SetPath("") })
	t.Setenv("CSL_PORT", "9424")

	if got, want := webBaseURL(newShared()), "http://127.0.0.1:9424"; got != want {
		t.Errorf("webBaseURL(newShared()) = %q, want %q", got, want)
	}
}

// TestReposDiscovered covers the three answers the check can give a
// configured machine: repos found (with the drops alongside the count),
// repos found and every one filtered out, and dirs that hold nothing.
func TestReposDiscovered(t *testing.T) {
	tests := []struct {
		name    string
		repos   map[string]string // dir under the walk root → origin remote
		hosts   string            // index.hosts YAML block, empty for none
		wantErr bool
		want    []string
	}{
		{
			name: "counts kept and dropped",
			repos: map[string]string{
				"keep": "git@github.com:org/keep.git",
				"off":  "git@git.example.com:org/off.git",
			},
			hosts: "index:\n  hosts:\n    - github.com\n",
			want:  []string{"1 repos", "1 dropped by index.hosts"},
		},
		{
			name:    "everything dropped fails with the pointer",
			repos:   map[string]string{"off": "git@git.example.com:org/off.git"},
			hosts:   "index:\n  hosts:\n    - github.com\n",
			wantErr: true,
			want:    []string{"1 git repos found", "csl repo --list --skipped"},
		},
		{
			name:    "no repos at all keeps the dirs wording",
			wantErr: true,
			want:    []string{"no git repos found under the dirs in"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for dir, remote := range tc.repos {
				writeRepo(t, filepath.Join(root, dir), remote)
			}
			configureCSL(t, "dirs:\n  - "+root+"\n"+tc.hosts)

			err := reposDiscovered(newShared()).Run(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Run() = nil, want a failure mentioning %v", tc.want)
				}
				if isSkip(err) {
					t.Fatalf("Run() skipped with %q, want a failure", err)
				}
			} else if err == nil || !isSkip(err) {
				t.Fatalf("Run() = %v, want an ok-with-detail skip", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("detail %q does not mention %q", err, want)
				}
			}
		})
	}
}

// TestIndexFreshnessNeverIndexed is the first-run contract: repos discovered
// and no index yet is where every machine starts, so freshness has nothing to
// compare and says so instead of reporting every repo as stale.
func TestIndexFreshnessNeverIndexed(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, filepath.Join(root, "keep"), "git@github.com:org/keep.git")
	configureCSL(t, "dirs:\n  - "+root+"\n")

	err := indexFreshness(t.TempDir(), newShared()).Run(context.Background())
	if err == nil || !isSkip(err) {
		t.Fatalf("index-freshness with no index = %v, want a skip", err)
	}
	for _, want := range []string{"1 repos discovered", "none of them indexed yet", "csl index"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("skip note %q does not mention %q", err, want)
		}
	}
}

// TestIndexFreshnessNoRepos keeps the doctor report from blaming one cause
// twice: repos-discovered owns the empty-discovery failure, so freshness
// steps aside rather than failing for the same reason.
func TestIndexFreshnessNoRepos(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, filepath.Join(root, "off"), "git@git.example.com:org/off.git")
	configureCSL(t, "dirs:\n  - "+root+"\nindex:\n  hosts:\n    - github.com\n")

	err := indexFreshness(t.TempDir(), newShared()).Run(context.Background())
	if err == nil || !isSkip(err) {
		t.Fatalf("index-freshness with no discovered repos = %v, want a skip", err)
	}
	if want := "no repos to check"; err.Error() != want {
		t.Errorf("skip note = %q, want %q", err, want)
	}
}

// isSkip reports whether err is a doctor skip (an "ok" with a note) rather
// than a failure. The skip type is unexported, so the test asks Collect,
// which is where the distinction surfaces for real callers too.
func isSkip(err error) bool {
	report := doctor.Collect(context.Background(), []doctor.Check{
		{Name: "probe", Run: func(context.Context) error { return err }},
	})
	return report.Checks[0].Status == doctor.StatusSkipped
}

// configureCSL points csl at a config file holding yaml, under a fake HOME.
func configureCSL(t *testing.T, yaml string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CSL_CONFIG", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, yaml)
	config.SetPath(path)
	t.Cleanup(func() { config.SetPath("") })
}

// writeRepo fakes a checkout the walker will find: a .git directory with an
// origin URL, which is all the finder reads. No subprocess, so a doctor test
// costs a file write instead of a `git init`.
func writeRepo(t *testing.T, dir, remote string) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(gitDir, "config"), "[remote \"origin\"]\n\turl = "+remote+"\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReposDiscoveredNoDirsSkips pins the single-cause rule: a config that
// sets no dirs is config-loads' failure, and repos-discovered must not fail a
// second time claiming repos were sought under dirs that do not exist.
func TestReposDiscoveredNoDirsSkips(t *testing.T) {
	configureCSL(t, "index:\n  hosts:\n    - github.com\n")

	err := reposDiscovered(newShared()).Run(context.Background())
	if err == nil || !isSkip(err) {
		t.Fatalf("Run() = %v, want a skip that defers to config-loads", err)
	}
	if !strings.Contains(err.Error(), "config-loads") {
		t.Errorf("skip note %q does not point at config-loads", err)
	}
}

// TestCatalogDescriptors pins the check that makes a silently dropped
// descriptor visible: readable descriptors pass plainly, none at all passes
// with a note naming the files csl looks for, and a broken one fails naming
// the repo and the file.
func TestCatalogDescriptors(t *testing.T) {
	component := "apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: x\nspec:\n  owner: y\n"
	tests := []struct {
		name     string
		files    map[string]string // repo dir → descriptor content ("" for none)
		wantSkip bool
		wantErr  bool
		want     []string
	}{
		{
			name:  "readable descriptors pass",
			files: map[string]string{"a": component, "b": ""},
		},
		{
			name:     "no descriptors pass with a note",
			files:    map[string]string{"a": "", "b": ""},
			wantSkip: true,
			want:     []string{"none of 2 repos", "catalog-info.yaml", ".csl-catalog.yaml"},
		},
		{
			name:    "broken descriptor fails naming it",
			files:   map[string]string{"a": component, "b": "kind: Component\nmetadata: [oops\n"},
			wantErr: true,
			want:    []string{"1 of 2 repos", "org/b", "catalog-info.yaml"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for dir, content := range tc.files {
				writeRepo(t, filepath.Join(root, dir), "git@github.com:org/"+dir+".git")
				if content != "" {
					writeFile(t, filepath.Join(root, dir, "catalog-info.yaml"), content)
				}
			}
			configureCSL(t, "dirs:\n  - "+root+"\n")

			err := catalogDescriptors(newShared()).Run(context.Background())
			switch {
			case tc.wantErr:
				if err == nil || isSkip(err) {
					t.Fatalf("Run() = %v, want a failure", err)
				}
			case tc.wantSkip:
				if err == nil || !isSkip(err) {
					t.Fatalf("Run() = %v, want a skip with a note", err)
				}
			default:
				if err != nil {
					t.Fatalf("Run() = %v, want nil", err)
				}
				return
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("detail %q does not mention %q", err, want)
				}
			}
		})
	}

	t.Run("no repos defers to repos-discovered", func(t *testing.T) {
		configureCSL(t, "dirs:\n  - "+t.TempDir()+"\n")
		err := catalogDescriptors(newShared()).Run(context.Background())
		if err == nil || !isSkip(err) {
			t.Fatalf("Run() = %v, want a skip", err)
		}
	})
}
