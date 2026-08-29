package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/worklog/internal/config"
)

// runConfig executes the config command against a config file that only this
// test can see, and returns its output split into the header line and the
// marshaled body.
func runConfig(t *testing.T, contents string, write bool) (header, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if write {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	c := (&app{configPath: path}).configCmd()
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(nil)
	if err := c.Execute(); err != nil {
		t.Fatalf("config command: %v, want nil error", err)
	}

	out := buf.String()
	head, rest, ok := strings.Cut(out, "\n")
	if !ok {
		t.Fatalf("output has no header line:\n%s", out)
	}
	if !strings.Contains(head, path) {
		t.Errorf("header %q, want it to name the resolved path %q", head, path)
	}
	return head, rest
}

func TestConfigMissingFile(t *testing.T) {
	header, body := runConfig(t, "", false)

	if !strings.Contains(header, "missing, defaults in use") {
		t.Errorf("header %q, want status %q", header, "missing, defaults in use")
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if len(got.Scan.LinearPrefixes) != 0 {
		t.Errorf("linear_prefixes = %v, want none: classification has no defaults",
			got.Scan.LinearPrefixes)
	}
	if len(got.Scan.RepoPathMarkers) == 0 {
		t.Error("got no repo_path_markers, want the path-shape defaults")
	}
}

func TestConfigLoadedFile(t *testing.T) {
	header, body := runConfig(t, "scan:\n  linear_prefixes: [ZZZ]\n", true)

	if !strings.Contains(header, "(loaded)") {
		t.Errorf("header %q, want status %q", header, "loaded")
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if want := []string{"ZZZ"}; len(got.Scan.LinearPrefixes) != 1 ||
		got.Scan.LinearPrefixes[0] != want[0] {
		t.Errorf("linear_prefixes = %v, want %v", got.Scan.LinearPrefixes, want)
	}
	if len(got.Scan.RepoPathMarkers) == 0 {
		t.Error("got no repo_path_markers, want the unset key filled from defaults")
	}
}

// TestConfigMalformedFile covers the one command that must survive a broken
// config: everything else in worklog fails on it, and this is where a user
// goes to find out why.
func TestConfigMalformedFile(t *testing.T) {
	header, body := runConfig(t, "scan: [", true)

	if !strings.Contains(header, "error:") {
		t.Errorf("header %q, want a parse error status", header)
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if len(got.Scan.RepoPathMarkers) == 0 {
		t.Error("got no repo_path_markers from a broken config, want the defaults printed anyway")
	}
}

func TestConfigLongDocumentsEveryKey(t *testing.T) {
	long := (&app{}).configCmd().Long
	keys := []string{
		"scan:",
		"linear_prefixes:",
		"personal_path_markers:",
		"internal_path_markers:",
		"checkout_roots:",
		"repo_path_markers:",
		"remote:",
		"url:",
		"push:",
	}
	for _, k := range keys {
		if !strings.Contains(long, k) {
			t.Errorf("Long help does not document %q", k)
		}
	}
	for _, override := range []string{"WORKLOG_CONFIG", "--config", "XDG_CONFIG_HOME"} {
		if !strings.Contains(long, override) {
			t.Errorf("Long help does not mention the %s override", override)
		}
	}
}

// TestRootHelpNamesItsDirectories keeps the root help answering "where does
// this keep things": the store root and the config file, with the variables
// that move each.
func TestRootHelpNamesItsDirectories(t *testing.T) {
	long := root().Long
	for _, want := range []string{"~/code/worklog", "WORKLOG_DIR", "WORKLOG_CONFIG", "--config"} {
		if !strings.Contains(long, want) {
			t.Errorf("root help does not mention %q:\n%s", want, long)
		}
	}
}

// TestRootConfigFlagOverridesEnv pins the precedence ADR-0011 fixes: the flag
// wins over the environment variable, which wins over the default path.
func TestRootConfigFlagOverridesEnv(t *testing.T) {
	t.Setenv("WORKLOG_CONFIG", "/tmp/from-env.yaml")

	c := root()
	if got := c.PersistentFlags().Lookup("config").DefValue; got != "/tmp/from-env.yaml" {
		t.Errorf("--config default = %q, want the environment value", got)
	}
	if err := c.PersistentFlags().Parse([]string{"--config", "/tmp/from-flag.yaml"}); err != nil {
		t.Fatal(err)
	}
	if got := c.PersistentFlags().Lookup("config").Value.String(); got != "/tmp/from-flag.yaml" {
		t.Errorf("--config = %q, want the flag to win", got)
	}
}
