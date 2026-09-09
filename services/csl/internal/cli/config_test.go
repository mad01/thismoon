package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// runConfigCmd points HOME at a temp dir so the test never reads the real
// config, optionally writes one there, and returns the command's output split
// into the header line and the marshaled body.
func runConfigCmd(t *testing.T, contents string, write bool) (header, body string) {
	t.Helper()
	home := t.TempDir()
	isolateConfigEnv(t, home)
	if write {
		dir := filepath.Join(home, ".config", "csl")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	if err := runConfig(cmd, nil); err != nil {
		t.Fatalf("config command: %v, want nil error", err)
	}

	head, rest, ok := strings.Cut(buf.String(), "\n")
	if !ok {
		t.Fatalf("output has no header line:\n%s", buf.String())
	}
	return head, rest
}

func TestConfigMissingFile(t *testing.T) {
	header, body := runConfigCmd(t, "", false)

	wantPath := filepath.Join(os.Getenv("HOME"), ".config", "csl", "config.yaml")
	if !strings.Contains(header, wantPath) {
		t.Errorf("header %q, want it to name %q", header, wantPath)
	}
	if !strings.Contains(header, "missing, defaults in use") {
		t.Errorf("header %q, want status %q", header, "missing, defaults in use")
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if got.Sync.Concurrency != 8 {
		t.Errorf("sync.concurrency = %d, want the default 8", got.Sync.Concurrency)
	}
	if got.Daemon.IdleTimeoutMinutes != 10 {
		t.Errorf("daemon.idle_timeout_minutes = %d, want the default 10",
			got.Daemon.IdleTimeoutMinutes)
	}
	if got.Semantic.EmbedModel == "" || got.Semantic.Dim == 0 {
		t.Errorf("semantic model/dim = %q/%d, want the embedder defaults",
			got.Semantic.EmbedModel, got.Semantic.Dim)
	}
}

func TestConfigLoadedFile(t *testing.T) {
	header, body := runConfigCmd(t, "dirs:\n  - /tmp/repos\nsync:\n  concurrency: 3\n", true)

	if !strings.Contains(header, "(loaded)") {
		t.Errorf("header %q, want status %q", header, "loaded")
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if len(got.Dirs) != 1 || got.Dirs[0] != "/tmp/repos" {
		t.Errorf("dirs = %v, want [/tmp/repos]", got.Dirs)
	}
	if got.Sync.Concurrency != 3 {
		t.Errorf("sync.concurrency = %d, want the configured 3", got.Sync.Concurrency)
	}
	if got.Web.BaseURL != csl.DefaultBaseURL {
		t.Errorf(
			"web.base_url = %q, want the resolved default %q",
			got.Web.BaseURL,
			csl.DefaultBaseURL,
		)
	}
}

func TestConfigMalformedFile(t *testing.T) {
	header, body := runConfigCmd(t, "dirs: [", true)

	if !strings.Contains(header, "parse error:") {
		t.Errorf("header %q, want a parse error status", header)
	}

	var got config.Config
	if err := yaml.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not valid YAML: %v\n%s", err, body)
	}
	if got.Sync.Concurrency != 8 {
		t.Errorf("sync.concurrency = %d, want the default 8 printed despite the parse error",
			got.Sync.Concurrency)
	}
}

func TestConfigLongDocumentsEveryKey(t *testing.T) {
	keys := []string{
		"dirs:",
		"hooks:",
		"post_merge:",
		"sync:",
		"index:",
		"semantic:",
		"daemon:",
		"refresh:",
		"web:",
		// The settings with no YAML key of their own still have to be findable
		// here: the reference is what a fresh machine is configured from.
		"CSL_CONFIG",
		"CSL_PORT",
	}
	for _, k := range keys {
		if !strings.Contains(configCmd.Long, k) {
			t.Errorf("Long help does not document %q", k)
		}
	}
}
