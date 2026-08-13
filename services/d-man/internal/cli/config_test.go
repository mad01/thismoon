package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/mad01/thismoon/services/d-man/internal/config"
)

// TestWriteEffectiveConfig pins the header status for each way a routes file
// can resolve, and that the body is always decodable TOML — a broken file
// reports the parse error and still prints the defaults d-man falls back to.
func TestWriteEffectiveConfig(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.toml")
	valid := write(
		t,
		dir,
		"routes.toml",
		"suffix = \"local\"\n\n[[route]]\n  name = \"csl\"\n  port = 7420\n",
	)
	broken := write(t, dir, "broken.toml", "suffix = \n")

	tests := []struct {
		name   string
		path   string
		status string
		suffix string
		routes int
	}{
		{"missing", missing, "missing, defaults in use", config.DefaultSuffix, 0},
		{"valid", valid, "loaded", "local", 1},
		{"malformed", broken, "parse error:", config.DefaultSuffix, 0},
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
			if got.Suffix != tt.suffix {
				t.Errorf("suffix = %q, want %q", got.Suffix, tt.suffix)
			}
			if len(got.Routes) != tt.routes {
				t.Errorf("routes = %d, want %d", len(got.Routes), tt.routes)
			}
		})
	}
}

// TestConfigLongDocumentsEveryKey guards the annotated example against a new
// config key landing without a line explaining it, and keeps the blocklist
// ordering rule (a blocklist below [[route]] is silently swallowed) in the help.
func TestConfigLongDocumentsEveryKey(t *testing.T) {
	for _, want := range []string{
		"suffix", "games_dir", "blocklist", "[[route]]",
		"name", "port", "target", "cname",
		"BEFORE the first", "DMAN_CONFIG", "/etc/d-man/routes.toml",
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
