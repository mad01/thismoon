package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteRegistryStatus pins the header for each way the registry can
// resolve. The source count is the whole payload of the command, so a
// miscount (or a swallowed parse error) has to fail here.
func TestWriteRegistryStatus(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent.yaml")
	two := write(t, dir, "registry.yaml", "sources:\n  - path: ~/code/a\n  - path: ~/code/b\n")
	one := write(t, dir, "single.yaml", "sources:\n  - path: ~/code/a\n")
	empty := write(t, dir, "empty.yaml", "sources: []\n")
	broken := write(t, dir, "broken.yaml", "sources: [\n")

	tests := []struct {
		name string
		path string
		want string
	}{
		{"missing", missing, "(missing)"},
		{"two sources", two, "(loaded, 2 sources)"},
		{"one source", one, "(loaded, 1 source)"},
		{"no sources", empty, "(loaded, 0 sources)"},
		{"malformed", broken, "(parse error:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeRegistryStatus(&buf, tt.path)
			got := buf.String()
			if !strings.HasPrefix(got, "registry: "+tt.path+" ") {
				t.Errorf("header %q does not name the registry path %q", got, tt.path)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("header = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// TestConfigLongDocumentsTheRegistry keeps the help pointing at the registry's
// keys, its override flag, and the command that schema-checks it.
func TestConfigLongDocumentsTheRegistry(t *testing.T) {
	for _, want := range []string{
		"sources", "path", "--registry", "~/.config/catalog/registry.yaml",
		"service-info.yaml", "catalog validate", "catalog list",
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
