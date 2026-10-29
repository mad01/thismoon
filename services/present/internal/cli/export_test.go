package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// -o names a file, or a directory the default file name goes into.
func TestExportPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "brief.html")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, out, want string }{
		{"no -o", "", "abc.html"},
		{"existing directory", dir, filepath.Join(dir, "abc.html")},
		{
			"missing directory with a separator",
			filepath.Join(dir, "new") + "/",
			filepath.Join(dir, "new", "abc.html"),
		},
		{"existing file", file, file},
		{"new file", filepath.Join(dir, "other.html"), filepath.Join(dir, "other.html")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exportPath(c.out, "abc.html"); got != c.want {
				t.Errorf("exportPath(%q) = %q, want %q", c.out, got, c.want)
			}
		})
	}
}
