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
	t.Setenv("WORKLOG_CONFIG", path)
	if write {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	c := configCmd()
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
	return head, rest
}

func TestConfigMissingFile(t *testing.T) {
	header, body := runConfig(t, "", false)

	if !strings.Contains(header, os.Getenv("WORKLOG_CONFIG")) {
		t.Errorf("header %q, want it to name the resolved path", header)
	}
	if !strings.Contains(header, "missing, defaults in use") {
		t.Errorf("header %q, want status %q", header, "missing, defaults in use")
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if len(got.Scan.LinearPrefixes) == 0 {
		t.Error("got no linear_prefixes with no config file, want the built-in default")
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

func TestConfigMalformedFile(t *testing.T) {
	header, body := runConfig(t, "scan: [", true)

	if !strings.Contains(header, "parse error:") {
		t.Errorf("header %q, want a parse error status", header)
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if len(got.Scan.LinearPrefixes) == 0 {
		t.Error("got no linear_prefixes from a broken config, want the defaults printed anyway")
	}
}

func TestConfigLongDocumentsEveryKey(t *testing.T) {
	long := configCmd().Long
	keys := []string{
		"scan:",
		"linear_prefixes:",
		"personal_path_markers:",
		"internal_path_markers:",
		"checkout_roots:",
		"repo_path_markers:",
	}
	for _, k := range keys {
		if !strings.Contains(long, k) {
			t.Errorf("Long help does not document %q", k)
		}
	}
	if !strings.Contains(long, "WORKLOG_CONFIG") {
		t.Error("Long help does not mention the $WORKLOG_CONFIG override")
	}
}
