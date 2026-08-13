package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/mad01/thismoon/services/deps/internal/config"
)

// TestWriteEffectiveConfig pins the header status for each way the discovery
// config can resolve, and that the body is always decodable TOML — a broken
// file reports the parse error and still prints the empty config deps would
// scan with.
func TestWriteEffectiveConfig(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.toml")
	valid := write(t, dir, "config.toml", "exclude_repos = [\"archive-old\"]\nexclude_paths = []\n")
	broken := write(t, dir, "broken.toml", "exclude_repos = \n")

	tests := []struct {
		name   string
		path   string
		status string
		repos  int
	}{
		{"missing", missing, "missing, defaults in use", 0},
		{"valid", valid, "loaded", 1},
		{"malformed", broken, "parse error:", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeEffectiveConfig(&buf, tt.path); err != nil {
				t.Fatalf("writeEffectiveConfig: %v", err)
			}
			header, body, ok := strings.Cut(buf.String(), "\n\n")
			if !ok {
				t.Fatalf("no header/body split in output:\n%s", buf.String())
			}
			if !strings.Contains(header, tt.path) {
				t.Errorf("header %q does not name the config path %q", header, tt.path)
			}
			if !strings.Contains(header, tt.status) {
				t.Errorf("header = %q, want status %q", header, tt.status)
			}

			var got config.Config
			if _, err := toml.Decode(body, &got); err != nil {
				t.Fatalf("body is not valid TOML: %v\n%s", err, body)
			}
			if len(got.ExcludeRepos) != tt.repos {
				t.Errorf("exclude_repos = %d entries, want %d", len(got.ExcludeRepos), tt.repos)
			}
			for _, key := range []string{"exclude_repos", "exclude_paths"} {
				if !strings.Contains(body, key) {
					t.Errorf("body omits %q:\n%s", key, body)
				}
			}
		})
	}
}

// TestConfigLongDocumentsEveryKey guards the annotated example against a new
// discovery key landing without a line explaining it.
func TestConfigLongDocumentsEveryKey(t *testing.T) {
	for _, want := range []string{
		"exclude_repos", "exclude_paths",
		"DEPS_CONFIG", "~/.config/deps/config.toml", "catalog registry",
	} {
		if !strings.Contains(configCmd.Long, want) {
			t.Errorf("config --help does not mention %q", want)
		}
	}
}

// write creates a file under dir and returns its path.
func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
