package server

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// The page shell is one file; shared mode rewrites the parts that only make
// sense on a machine with local .this sites and an index: the ⌘K control and
// the "← All" back link. Pin the markers the rewrite keys on as well, so an
// edit to shell.html cannot turn it into a silent no-op.
func TestPageShellPerMode(t *testing.T) {
	local := string(pageShell(ModeLocal))
	if !strings.Contains(local, `controls="cmdk,`) ||
		!strings.Contains(local, `back-label="← All"`) {
		t.Fatal("shell.html no longer carries the markers the shared rewrite keys on")
	}

	shared := string(pageShell(ModeShared))
	if strings.Contains(shared, "cmdk") {
		t.Error("shared shell still lists the cmdk control")
	}
	if !strings.Contains(shared, `controls="font,`) {
		t.Error("shared shell lost the controls that follow cmdk")
	}
	if !strings.Contains(shared, `back-label="← About"`) {
		t.Error("shared shell keeps the local back label")
	}
}

// Every shell links the themes page webkit serves, so a reader can pick a
// palette family from any present view, shared instances included.
func TestShellsLinkThemesPage(t *testing.T) {
	const link = `<a data-nav href="/webkit/themes">Themes</a>`
	for name, shell := range map[string][]byte{
		"page":         pageShell(ModeLocal),
		"page-shared":  pageShell(ModeShared),
		"index":        indexShellHTML,
		"shared-index": sharedIndexShellHTML,
	} {
		if !strings.Contains(string(shell), link) {
			t.Errorf("%s shell has no themes link", name)
		}
	}
	if strings.Contains(string(shellHTML), "var(--wg") {
		t.Error("shell.html reads a ramp token; the palette only guarantees roles")
	}
}

func TestSharedChromeHasNoSitePicker(t *testing.T) {
	f := setupShared(t)
	p, err := f.raw.Create(t.Context(), store.Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/p/" + p.ID} {
		code, body := get(t, f.ts.URL+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, code)
		}
		if strings.Contains(body, "cmdk") {
			t.Errorf("GET %s on a shared instance serves the cmdk control", path)
		}
	}

	ts, st := setup(t)
	local := createLocal(t, st, "Local")
	if _, body := get(t, ts.URL+"/p/"+local.ID); !strings.Contains(body, "cmdk") {
		t.Error("local page shell lost the cmdk control")
	}
}

// The deck view opens with the read-aloud controls hidden and the brief with
// them shown, each remembering its own choice: both shells list the header's
// audio toggle, and the deck shell names its own storage key and default on
// <html>, which webkit's boot.js and the header read, while the brief shell
// names neither and so gets webkit's defaults.
func TestDeckShellAudioDefault(t *testing.T) {
	const marker = `<html lang="en" data-audio-key="webkit-audio-deck" data-audio-default="off">`
	for name, mode := range map[string]Mode{"local": ModeLocal, "shared": ModeShared} {
		brief := string(pageShell(mode))
		deck := string(deckShell(pageShell(mode)))
		if !strings.Contains(brief, "size,audio,speed,") {
			t.Errorf("%s: shell header does not list the audio toggle beside speed", name)
		}
		if strings.Contains(brief, "data-audio") {
			t.Errorf("%s: brief shell names an audio key or default; it must take webkit's", name)
		}
		if !strings.Contains(deck, marker) {
			t.Errorf("%s: deck shell lacks the audio marker %q", name, marker)
		}
		if strings.Count(deck, "data-audio-key") != 1 {
			t.Errorf(
				"%s: deck shell names the audio key %d times",
				name,
				strings.Count(deck, "data-audio-key"),
			)
		}
	}

	ts, st := setup(t)
	both, err := st.Create(t.Context(), store.Draft{
		Title: "Both", Content: "<p>brief</p>", Deck: "<h1 class=\"brief-title\">Both</h1>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, body := get(t, ts.URL+"/p/"+both.ID+"/deck"); !strings.Contains(body, marker) {
		t.Error("GET /p/{id}/deck does not serve the deck shell")
	}
	if _, body := get(t, ts.URL+"/p/"+both.ID); strings.Contains(body, "data-audio") {
		t.Error("GET /p/{id} serves the deck's audio default")
	}
}

// A panel accent named terracotta renders as var(--primary) and webkit no
// longer declares --terracotta, but pages stored before that change still
// carry var(--terracotta), and a shared instance never rerenders them. Every
// page shell declares the alias as primary so those accent bars keep
// resolving (MAD-371).
func TestShellsDeclareTerracottaAlias(t *testing.T) {
	const rule = `:root { --terracotta: var(--primary); }`
	for name, shell := range map[string][]byte{
		"page":        pageShell(ModeLocal),
		"page-shared": pageShell(ModeShared),
		"deck":        deckShell(pageShell(ModeLocal)),
		"deck-shared": deckShell(pageShell(ModeShared)),
	} {
		if !strings.Contains(string(shell), rule) {
			t.Errorf("%s shell lacks the terracotta compatibility rule %q", name, rule)
		}
	}
}

var (
	versionVar   = regexp.MustCompile(`(?m)^([A-Z0-9_]+_VERSION)="([^"]+)"$`)
	assetFileVar = regexp.MustCompile(`(?m)^[a-z0-9_]+_file="\$ASSETS/js/([^"]+)"$`)
	varRef       = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
	shellJSSrc   = regexp.MustCompile(`src="/assets/js/([^"]+)"`)
)

// The page shell loads each vendored script by a versioned filename that
// scripts/cache-assets.sh writes into the workdir (docs/adr/0022). Pin the
// two to each other, so a version bump in one place cannot leave the shell
// asking for a file the script never fetches, or the script fetching one no
// page loads.
func TestShellLoadsEveryCachedScript(t *testing.T) {
	script, err := os.ReadFile("../../scripts/cache-assets.sh")
	if err != nil {
		t.Fatal(err)
	}
	fetched := cachedScripts(t, string(script))
	if len(fetched) == 0 {
		t.Fatal("cache-assets.sh declares no $ASSETS/js file; the pattern no longer matches")
	}
	loaded := map[string]bool{}
	for _, m := range shellJSSrc.FindAllStringSubmatch(string(shellHTML), -1) {
		loaded[m[1]] = true
	}
	for name := range fetched {
		if !loaded[name] {
			t.Errorf("cache-assets.sh fetches %s but shell.html never loads /assets/js/%s", name, name)
		}
	}
	for name := range loaded {
		if !fetched[name] {
			t.Errorf("shell.html loads /assets/js/%s but cache-assets.sh never fetches it", name)
		}
	}
}

// cachedScripts resolves the $ASSETS/js filenames cache-assets.sh declares,
// expanding the *_VERSION variables they name.
func cachedScripts(t *testing.T, script string) map[string]bool {
	t.Helper()
	versions := map[string]string{}
	for _, m := range versionVar.FindAllStringSubmatch(script, -1) {
		versions[m[1]] = m[2]
	}
	files := map[string]bool{}
	for _, m := range assetFileVar.FindAllStringSubmatch(script, -1) {
		name := varRef.ReplaceAllStringFunc(m[1], func(ref string) string {
			key := varRef.FindStringSubmatch(ref)[1]
			version, ok := versions[key]
			if !ok {
				t.Errorf("cache-assets.sh names ${%s} in %s but never sets it", key, m[1])
			}
			return version
		})
		files[name] = true
	}
	return files
}
