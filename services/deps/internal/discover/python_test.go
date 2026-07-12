package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// Mirrors the real speak-tts requirements.txt: comments, an extras pin, a
// plain pin, and an unpinned name that must be skipped.
const requirementsTxt = `# engine deps — pins are load-bearing
mlx-audio[server]==0.4.3
mlx==0.31.1
misaki[en]
`

func TestParseRequirements(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requirements.txt")
	if err := os.WriteFile(path, []byte(requirementsTxt), 0o644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := parseRequirements(path)
	if err != nil {
		t.Fatalf("parseRequirements: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2: %+v", len(pkgs), pkgs)
	}
	for _, p := range pkgs {
		if p.Ecosystem != "PyPI" || !p.Direct || !p.Imported || p.ManifestPath != path {
			t.Errorf("%s = %+v, want PyPI direct imported with manifest path", p.Name, p)
		}
	}
	if pkgs[0].Name != "mlx-audio" || pkgs[0].Version != "0.4.3" {
		t.Errorf("pkgs[0] = %+v, want mlx-audio 0.4.3 (extras stripped)", pkgs[0])
	}
	if pkgs[1].Name != "mlx" || pkgs[1].Version != "0.31.1" {
		t.Errorf("pkgs[1] = %+v, want mlx 0.31.1", pkgs[1])
	}
}

func TestParseRequirementLine(t *testing.T) {
	tests := []struct {
		line    string
		name    string
		version string
		ok      bool
	}{
		{"requests==2.31.0", "requests", "2.31.0", true},
		{"  requests == 2.31.0  ", "requests", "2.31.0", true},
		{"mlx-audio[server]==0.4.3", "mlx-audio", "0.4.3", true},
		{"requests==2.31.0  # inline comment", "requests", "2.31.0", true},
		{`requests==2.31.0 ; python_version < "3.12"`, "requests", "2.31.0", true},
		{"requests==2.31.0,<3", "requests", "2.31.0", true},
		// PEP 503 normalization: OSV keys PyPI on the normalized name.
		{"Django==4.2", "django", "4.2", true},
		{"zope.interface==6.0", "zope-interface", "6.0", true},
		{"typing_extensions==4.9.0", "typing-extensions", "4.9.0", true},

		{"", "", "", false},
		{"# comment only", "", "", false},
		{"misaki[en]", "", "", false},              // unpinned: no version to check
		{"requests>=2.0", "", "", false},           // range, not a pin
		{"requests~=2.31", "", "", false},          // compatible release, not exact
		{"requests>=2,==2.*", "", "", false},       // operator before the ==
		{"requests===2.31.0", "", "", false},       // arbitrary equality
		{"requests==2.*", "", "", false},           // wildcard, not exact
		{"-r other.txt", "", "", false},            // pip option
		{"--index-url https://x", "", "", false},   // pip option
		{"pkg @ https://x/pkg.whl", "", "", false}, // direct reference
	}
	for _, tt := range tests {
		name, version, ok := parseRequirementLine(tt.line)
		if name != tt.name || version != tt.version || ok != tt.ok {
			t.Errorf("parseRequirementLine(%q) = %q, %q, %v; want %q, %q, %v",
				tt.line, name, version, ok, tt.name, tt.version, tt.ok)
		}
	}
}

func TestPythonDiscoverNoManifest(t *testing.T) {
	pkgs, err := Python{}.Discover(t.TempDir(), Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if pkgs != nil {
		t.Fatalf("got %+v, want nil for a repo with no requirements.txt", pkgs)
	}
}
