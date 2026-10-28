package discover

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/jsdelivr"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// published stands in for cytoscape 3.31.0's dist/cytoscape.min.js: the fake
// API answers its hash with the recorded jsDelivr answer for the real file.
var published = []byte("/* cytoscape 3.31.0, as published */\n")

func sumOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// jsdelivrAPI fakes data.jsdelivr.com from the recorded answers: the cytoscape
// hit for the hash of published, the recorded 404 for every other hash, or 503
// for every request when failing is set. It records the hashes asked for.
type jsdelivrAPI struct {
	mu      sync.Mutex
	asked   []string
	failing bool
}

func (a *jsdelivrAPI) serve(t *testing.T) *jsdelivr.Client {
	t.Helper()
	hit, err := os.ReadFile("../jsdelivr/testdata/lookup-cytoscape-3.31.0.json")
	if err != nil {
		t.Fatal(err)
	}
	miss, err := os.ReadFile("../jsdelivr/testdata/lookup-miss.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sum := strings.TrimPrefix(r.URL.Path, "/v1/lookup/hash/")
		a.mu.Lock()
		a.asked = append(a.asked, sum)
		failing := a.failing
		a.mu.Unlock()
		switch {
		case failing:
			w.WriteHeader(http.StatusServiceUnavailable)
		case sum == sumOf(published):
			_, _ = w.Write(hit)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(miss)
		}
	}))
	t.Cleanup(srv.Close)
	return jsdelivr.NewWithURL(srv.URL)
}

// hashes returns the hashes asked for so far.
func (a *jsdelivrAPI) hashes() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.asked...)
}

const staticDir = "services/catalog/internal/web/assets/static/"

func TestVendoredJSNamesAPublishedFileByItsHash(t *testing.T) {
	api := &jsdelivrAPI{}
	root := t.TempDir()
	path := writeRepoFile(t, root, staticDir+"cytoscape-3.31.0.min.js", published)

	pkgs, err := VendoredJS{Lookup: api.serve(t)}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := Package{
		Ecosystem:    "npm",
		Name:         "cytoscape",
		Version:      "3.31.0",
		ManifestPath: path,
		Direct:       true,
		Imported:     true,
	}
	if len(pkgs) != 1 || pkgs[0].Name != want.Name || pkgs[0].Version != want.Version ||
		pkgs[0].ManifestPath != want.ManifestPath || !pkgs[0].Direct || !pkgs[0].Imported ||
		len(pkgs[0].Advisories) != 0 || pkgs[0].NameFromFile {
		t.Errorf("pkgs = %+v, want [%+v]", pkgs, want)
	}
}

func TestVendoredJSHashesFilesUnderVendor(t *testing.T) {
	api := &jsdelivrAPI{}
	root := t.TempDir()
	path := writeRepoFile(t, root, "web/static/vendor/cytoscape-3.31.0.min.js", published)

	pkgs, err := VendoredJS{Lookup: api.serve(t)}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "cytoscape" || pkgs[0].Version != "3.31.0" ||
		pkgs[0].ManifestPath != path {
		t.Errorf("pkgs = %+v, want cytoscape@3.31.0 from %s", pkgs, path)
	}
}

func TestVendoredJSFlagsAChangedFile(t *testing.T) {
	api := &jsdelivrAPI{}
	root := t.TempDir()
	changed := append([]byte("window.steal = 1;\n"), published...)
	writeRepoFile(t, root, staticDir+"cytoscape-3.31.0.min.js", changed)

	pkgs, err := VendoredJS{Lookup: api.serve(t)}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("pkgs = %+v, want the changed file named from its file name", pkgs)
	}
	p := pkgs[0]
	if p.Name != "cytoscape" || p.Version != "3.31.0+sha256."+sumOf(changed)[:12] {
		t.Errorf(
			"name@version = %s@%s, want cytoscape@3.31.0 with the hash prefix",
			p.Name,
			p.Version,
		)
	}
	if strings.ContainsAny(p.Version, "@:") {
		t.Errorf("version %q holds a key separator", p.Version)
	}
	if !p.NameFromFile {
		t.Error("NameFromFile = false, want true for a name taken from the file name")
	}
	if len(p.Advisories) != 1 || p.Advisories[0].ID != "deps-vendored-modified" ||
		p.Advisories[0].Kind != store.KindSignal || p.Advisories[0].Summary == "" {
		t.Errorf("advisories = %+v, want the deps-vendored-modified signal", p.Advisories)
	}
}

func TestVendoredJSHashesOnlyVendoredLookingFiles(t *testing.T) {
	api := &jsdelivrAPI{}
	root := t.TempDir()
	writeRepoFile(t, root, staticDir+"app.js", []byte("first party"))
	writeRepoFile(t, root, staticDir+"app.css", []byte("first party"))
	writeRepoFile(t, root, "web/dist/chunk-1.2.3.min.js", []byte("not under static"))
	writeRepoFile(t, root, "static/node_modules/x/x-1.0.0.min.js", []byte("skipped dir"))
	bundle := writeRepoFile(t, root, "static/js/bundle.min.js", []byte("own build"))

	pkgs, err := VendoredJS{Lookup: api.serve(t)}.Discover(root, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("pkgs = %+v, want none (an unversioned miss has no name)", pkgs)
	}
	if asked := api.hashes(); len(asked) != 1 || asked[0] != sumOf([]byte("own build")) {
		t.Errorf("asked = %v, want only %s's hash", asked, bundle)
	}
}

func TestVendoredJSFailsTheRepoWhenALookupFails(t *testing.T) {
	api := &jsdelivrAPI{failing: true}
	root := t.TempDir()
	path := writeRepoFile(t, root, staticDir+"cytoscape-3.31.0.min.js", published)

	pkgs, err := VendoredJS{Lookup: api.serve(t)}.Discover(root, Options{})
	if err == nil || pkgs != nil {
		t.Fatalf("Discover = %+v, %v; want an error and no packages", pkgs, err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file", err)
	}
}

// A GitHub match has no npm name, so the file name supplies it, but the
// version is jsDelivr's: here 3.7.0's file sits under 3.7.1's name.
func TestVendoredPackageNamesAGitHubMatchByFileNameAndJSDelivrVersion(t *testing.T) {
	gh := jsdelivr.File{
		Type:    "gh",
		Name:    "jquery/jquery",
		Version: "v3.7.0",
		Path:    "/dist/jquery.min.js",
	}
	p, ok := vendoredPackage("/r/static/jquery-3.7.1.min.js", sumOf(nil), gh, true)
	if !ok || p.Name != "jquery" || p.Version != "3.7.0" || len(p.Advisories) != 0 ||
		!p.NameFromFile {
		t.Errorf(
			"vendoredPackage = %+v, %v; want npm jquery@3.7.0 named from its file, no signal",
			p, ok,
		)
	}
}

func TestVendoredCandidate(t *testing.T) {
	tests := map[string]bool{
		"svc/assets/static/cytoscape-3.31.0.min.js":   true,
		"static/js/cytoscape-dagre-2.5.0.js":          true,
		"static/elk-0.12.0.bundled.js":                true,
		"static/foo-v1.0.0-rc.1.min.js":               true,
		"static/vendor.min.js":                        true,
		"svc/assets/static/app.js":                    false,
		"svc/assets/static/cytoscape-3.31.0.min.css":  false,
		"svc/assets/cytoscape-3.31.0.min.js":          false,
		"staticfiles/cytoscape-3.31.0.min.js":         false,
		"svc/assets/static/cytoscape-3.31.0.min.js.x": false,
	}
	for rel, want := range tests {
		if got := vendoredCandidate(rel); got != want {
			t.Errorf("vendoredCandidate(%q) = %v, want %v", rel, got, want)
		}
	}
}
