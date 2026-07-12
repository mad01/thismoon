package discover

import (
	"os"
	"path/filepath"
	"testing"
)

// Mirrors the fleet's Package.resolved v3 shape: versioned remote pins, a
// branch pin without a version (mathpet's mlx-swift-lm), and a local pin.
const swiftResolvedJSON = `{
  "originHash" : "abc123",
  "pins" : [
    {
      "identity" : "purchases-ios",
      "kind" : "remoteSourceControl",
      "location" : "https://github.com/RevenueCat/purchases-ios.git",
      "state" : { "revision" : "3ea11388", "version" : "5.41.0" }
    },
    {
      "identity" : "zipfoundation",
      "kind" : "remoteSourceControl",
      "location" : "https://github.com/weichsel/ZIPFoundation.git",
      "state" : { "revision" : "22787ffb", "version" : "0.9.20" }
    },
    {
      "identity" : "mlx-swift-lm",
      "kind" : "remoteSourceControl",
      "location" : "https://github.com/ml-explore/mlx-swift-lm.git",
      "state" : { "branch" : "main", "revision" : "deadbeef" }
    },
    {
      "identity" : "local-kit",
      "kind" : "fileSystem",
      "location" : "../LocalKit",
      "state" : { "version" : "1.0.0" }
    }
  ],
  "version" : 3
}`

func TestParseSwiftResolved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Package.resolved")
	if err := os.WriteFile(path, []byte(swiftResolvedJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := parseSwiftResolved(path)
	if err != nil {
		t.Fatalf("parseSwiftResolved: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2 (branch and local pins skipped): %+v", len(pkgs), pkgs)
	}
	for _, p := range pkgs {
		if p.Ecosystem != "SwiftURL" || !p.Direct || !p.Imported || p.ManifestPath != path {
			t.Errorf("%s = %+v, want SwiftURL direct imported with manifest path", p.Name, p)
		}
	}
	if pkgs[0].Name != "github.com/RevenueCat/purchases-ios" || pkgs[0].Version != "5.41.0" {
		t.Errorf("pkgs[0] = %+v, want github.com/RevenueCat/purchases-ios 5.41.0", pkgs[0])
	}
	if pkgs[1].Name != "github.com/weichsel/ZIPFoundation" || pkgs[1].Version != "0.9.20" {
		t.Errorf("pkgs[1] = %+v, want github.com/weichsel/ZIPFoundation 0.9.20", pkgs[1])
	}
}

func TestSwiftPackageName(t *testing.T) {
	tests := []struct {
		location string
		want     string
	}{
		{"https://github.com/vapor/vapor.git", "github.com/vapor/vapor"},
		{"https://github.com/vapor/vapor", "github.com/vapor/vapor"},
		{"http://github.com/vapor/vapor.git", "github.com/vapor/vapor"},
		{"git@github.com:vapor/vapor.git", "github.com/vapor/vapor"},
		{"ssh://git@github.com/vapor/vapor.git", "github.com/vapor/vapor"},
		{"https://github.com/vapor/vapor/", "github.com/vapor/vapor"},
	}
	for _, tt := range tests {
		if got := swiftPackageName(tt.location); got != tt.want {
			t.Errorf("swiftPackageName(%q) = %q, want %q", tt.location, got, tt.want)
		}
	}
}

func TestSwiftDiscoverNoManifest(t *testing.T) {
	pkgs, err := Swift{}.Discover(t.TempDir(), Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if pkgs != nil {
		t.Fatalf("got %+v, want nil for a repo with no Package.resolved", pkgs)
	}
}
