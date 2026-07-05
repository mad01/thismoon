package discover

import (
	"strings"
	"testing"
)

// goListStream is a canned `go list -m -json all` stream: the main module, one
// direct dep, one indirect dep, and a module replaced with a local path (no
// version). parseGoList must keep the two real deps, mark direct vs indirect,
// strip the leading v, and drop both the main module and the local replace.
const goListStream = `{
	"Path": "github.com/mad01/thismoon/services/deps",
	"Main": true
}
{
	"Path": "github.com/spf13/cobra",
	"Version": "v1.10.2"
}
{
	"Path": "github.com/segmentio/asm",
	"Version": "v1.1.3",
	"Indirect": true
}
{
	"Path": "github.com/mad01/thismoon/webkit",
	"Version": "v0.1.0",
	"Replace": {
		"Path": "../webkit",
		"Version": ""
	}
}`

func TestParseGoList(t *testing.T) {
	// nil imported set + importedOK false → reachability unavailable, every dep
	// fails open to Imported=true.
	pkgs, err := parseGoList(strings.NewReader(goListStream), "/repo/go.mod", nil, false)
	if err != nil {
		t.Fatalf("parseGoList: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2: %+v", len(pkgs), pkgs)
	}

	byName := map[string]Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}

	cobra, ok := byName["github.com/spf13/cobra"]
	if !ok {
		t.Fatal("missing cobra")
	}
	if cobra.Version != "1.10.2" {
		t.Errorf("cobra version = %q, want 1.10.2 (v stripped)", cobra.Version)
	}
	if !cobra.Direct {
		t.Error("cobra should be direct")
	}
	if cobra.Ecosystem != "Go" {
		t.Errorf("ecosystem = %q, want Go", cobra.Ecosystem)
	}
	if cobra.ManifestPath != "/repo/go.mod" {
		t.Errorf("manifest = %q, want /repo/go.mod", cobra.ManifestPath)
	}

	asm, ok := byName["github.com/segmentio/asm"]
	if !ok {
		t.Fatal("missing asm")
	}
	if asm.Direct {
		t.Error("asm should be indirect")
	}

	if _, ok := byName["github.com/mad01/thismoon/webkit"]; ok {
		t.Error("locally-replaced module (no version) should be dropped")
	}
	if _, ok := byName["github.com/mad01/thismoon/services/deps"]; ok {
		t.Error("main module should be dropped")
	}

	// Fail-open: with no reachability info, both real deps are imported.
	for _, p := range pkgs {
		if !p.Imported {
			t.Errorf("%s: Imported=false, want true when reachability is unavailable", p.Name)
		}
	}
}

// goListDepsStream is a canned `go list -deps -json ./...` package stream: two
// packages from cobra (imported), one stdlib package (no Module), and a package
// from a replaced module reported under its original path with a Replace block.
// asm is absent — it's in the module graph but never imported.
const goListDepsStream = `{
	"ImportPath": "fmt",
	"Standard": true
}
{
	"ImportPath": "github.com/spf13/cobra",
	"Module": { "Path": "github.com/spf13/cobra" }
}
{
	"ImportPath": "github.com/spf13/cobra/doc",
	"Module": { "Path": "github.com/spf13/cobra" }
}
{
	"ImportPath": "github.com/mad01/thismoon/webkit",
	"Module": { "Path": "github.com/mad01/thismoon/webkit", "Replace": { "Path": "../webkit" } }
}`

func TestParseGoListDeps(t *testing.T) {
	set, ok := parseGoListDeps(strings.NewReader(goListDepsStream))
	if !ok {
		t.Fatal("parseGoListDeps: ok=false")
	}
	if _, want := set["github.com/spf13/cobra"]; !want {
		t.Error("cobra should be in the import set")
	}
	if _, want := set["github.com/mad01/thismoon/webkit"]; !want {
		t.Error("replaced module original path should be in the import set")
	}
	if _, want := set["../webkit"]; !want {
		t.Error("replaced module replacement path should be in the import set")
	}
	if _, in := set[""]; in {
		t.Error("stdlib package (no Module) leaked an empty path into the set")
	}
}

// TestParseGoListMarksUnimported confirms a module in the graph but absent from
// the import set (segmentio/asm here) is tagged Imported=false, while an
// imported module (cobra) stays true.
func TestParseGoListMarksUnimported(t *testing.T) {
	imported := map[string]struct{}{"github.com/spf13/cobra": {}}
	pkgs, err := parseGoList(strings.NewReader(goListStream), "/repo/go.mod", imported, true)
	if err != nil {
		t.Fatalf("parseGoList: %v", err)
	}
	byName := map[string]Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if !byName["github.com/spf13/cobra"].Imported {
		t.Error("cobra is imported, want Imported=true")
	}
	if byName["github.com/segmentio/asm"].Imported {
		t.Error("asm is graph-only (not imported), want Imported=false")
	}
}
