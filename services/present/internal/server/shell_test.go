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

// The shared shell tells the browser it is on a shared instance; a local
// shell says nothing. The deck shell wraps the same shell, so a shared deck
// carries the mode and the deck's audio default on the one <html> tag.
func TestSharedShellMarksTheMode(t *testing.T) {
	const mode = `data-present-mode="shared"`
	local, shared := string(pageShell(ModeLocal)), string(pageShell(ModeShared))
	if !strings.Contains(local, `<html lang="en">`) {
		t.Fatal("shell.html no longer carries the <html> tag the mode rewrite keys on")
	}
	if strings.Contains(local, "data-present-mode") ||
		strings.Contains(string(deckShell(pageShell(ModeLocal))), "data-present-mode") {
		t.Error("a local shell names a mode")
	}
	if !strings.Contains(shared, `<html lang="en" `+mode+`>`) {
		t.Errorf("shared brief shell lacks %s on <html>", mode)
	}
	deck := string(deckShell(pageShell(ModeShared)))
	want := `<html lang="en" data-audio-key="webkit-audio-deck" data-audio-default="off" ` +
		mode + `>`
	if !strings.Contains(deck, want) {
		t.Errorf("shared deck shell lacks %q", want)
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
	const marker = `<html lang="en" data-audio-key="webkit-audio-deck" data-audio-default="off"`
	for name, mode := range map[string]Mode{"local": ModeLocal, "shared": ModeShared} {
		brief := string(pageShell(mode))
		deck := string(deckShell(pageShell(mode)))
		if !strings.Contains(brief, "size,audio,speed,") {
			t.Errorf("%s: shell header does not list the audio toggle beside speed", name)
		}
		if namesAudioKeyOrDefault(brief) {
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
	if _, body := get(t, ts.URL+"/p/"+both.ID); namesAudioKeyOrDefault(body) {
		t.Error("GET /p/{id} serves the deck's audio default")
	}
}

// namesAudioKeyOrDefault reports whether a page sets either attribute webkit
// reads its audio choice from. The brief's stylesheet may still match the
// data-audio state webkit sets on <html>; only these two name a key or default.
func namesAudioKeyOrDefault(page string) bool {
	return strings.Contains(page, "data-audio-key") || strings.Contains(page, "data-audio-default")
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

// The brief's reading styles are one block of shell.html between two marker
// comments. The block declares the type scale on #root and the strong ink on
// :root, reads webkit's --wk-measure rather than a measure of its own, and
// scopes every other selector to .brief:not(.deck), so no rule of it reaches
// the deck.
func TestBriefStylesStayInTheBrief(t *testing.T) {
	const (
		begin = "/* The brief's reading styles."
		end   = "/* End of the brief's reading styles. */"
		scope = ".brief:not(.deck)"
	)
	shell := string(shellHTML)
	from, to := strings.Index(shell, begin), strings.Index(shell, end)
	if from < 0 || to < from {
		t.Fatalf("shell.html lacks the brief's block between %q and %q", begin, end)
	}
	block := shell[from:to]
	for _, token := range []string{
		"--ink-strong:", "--fs-micro:", "--fs-small:", "--fs-body:",
		"--fs-h3:", "--fs-lede:", "--fs-h2:", "--fs-title:",
	} {
		if !strings.Contains(block, token) {
			t.Errorf("the brief's block does not declare %s", strings.TrimSuffix(token, ":"))
		}
	}
	if strings.Contains(shell, "--measure:") {
		t.Error("shell.html declares a measure of its own; the brief reads webkit's --wk-measure")
	}
	if !strings.Contains(block, "var(--wk-measure)") {
		t.Error("the brief's block never reads --wk-measure")
	}
	tokenRules := 0
	for _, rule := range cssRules(t, block) {
		if rule.selector == ":root" || rule.selector == "#root" {
			tokenRules++
			if !declaresOnlyTokens(rule.body) {
				t.Errorf(
					"the token rule %s sets more than custom properties: %q",
					rule.selector,
					rule.body,
				)
			}
			continue
		}
		for _, sel := range splitSelectors(rule.selector) {
			if !strings.HasPrefix(sel, scope) {
				t.Errorf("selector %q in the brief's block lacks the %s scope", sel, scope)
			}
		}
	}
	if tokenRules != 2 {
		t.Errorf("the brief's block has %d token rules, want the :root and #root ones", tokenRules)
	}
}

// cssRule is one style rule: its selector list and its declarations.
type cssRule struct{ selector, body string }

var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// cssRules lists the style rules of a stylesheet, those inside @media and
// @container blocks included. Declarations hold no braces, so a rule's body
// ends at the first closing brace.
func cssRules(t *testing.T, sheet string) []cssRule {
	t.Helper()
	sheet = cssComment.ReplaceAllString(sheet, "")
	var rules []cssRule
	for sheet = strings.TrimSpace(sheet); sheet != ""; sheet = strings.TrimSpace(sheet) {
		if strings.HasPrefix(sheet, "}") {
			sheet = sheet[1:] // the end of an at-rule block
			continue
		}
		open := strings.Index(sheet, "{")
		if open < 0 {
			t.Fatalf("stray text after the last rule: %q", sheet)
		}
		prelude := strings.Join(strings.Fields(sheet[:open]), " ")
		sheet = sheet[open+1:]
		if strings.HasPrefix(prelude, "@") {
			continue // its rules follow, up to its closing brace
		}
		closing := strings.Index(sheet, "}")
		if closing < 0 {
			t.Fatalf("rule %q never closes", prelude)
		}
		rules = append(rules, cssRule{selector: prelude, body: strings.TrimSpace(sheet[:closing])})
		sheet = sheet[closing+1:]
	}
	return rules
}

// splitSelectors splits a selector list at the commas outside parentheses.
func splitSelectors(list string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(list[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(list[start:]))
}

// declaresOnlyTokens reports whether every declaration sets a custom property.
func declaresOnlyTokens(body string) bool {
	for _, decl := range strings.Split(body, ";") {
		if decl = strings.TrimSpace(decl); decl != "" && !strings.HasPrefix(decl, "--") {
			return false
		}
	}
	return true
}

var (
	versionVar   = regexp.MustCompile(`(?m)^([A-Z0-9_]+_VERSION)="([^"]+)"$`)
	assetFileVar = regexp.MustCompile(`(?m)^[a-z0-9_]+_file="\$ASSETS/((?:js|css)/[^"]+)"$`)
	varRef       = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
	// shellAssets are the two ways the shell loads a cached file: a script
	// by src, a stylesheet by href.
	shellAssets = []*regexp.Regexp{
		regexp.MustCompile(`src="/assets/(js/[^"]+)"`),
		regexp.MustCompile(`href="/assets/(css/[^"]+)"`),
	}
)

// The page shell loads each vendored script and stylesheet by a versioned
// filename that scripts/cache-assets.sh writes into the workdir (docs/adr/0022).
// Pin the two to each other, so a version bump in one place cannot leave the
// shell asking for a file the script never fetches, or the script fetching one
// no page loads. The font stylesheet, fonts.css, has no version in its name
// but pairs the same way: the script writes it and the shell links it.
func TestShellLoadsEveryCachedAsset(t *testing.T) {
	script, err := os.ReadFile("../../scripts/cache-assets.sh")
	if err != nil {
		t.Fatal(err)
	}
	fetched := cachedAssets(t, string(script))
	if len(fetched) == 0 {
		t.Fatal(
			"cache-assets.sh declares no $ASSETS/js or $ASSETS/css file; the pattern no longer matches",
		)
	}
	loaded := map[string]bool{}
	for _, re := range shellAssets {
		for _, m := range re.FindAllStringSubmatch(string(shellHTML), -1) {
			loaded[m[1]] = true
		}
	}
	for name := range fetched {
		if !loaded[name] {
			t.Errorf("cache-assets.sh fetches %s but shell.html never loads /assets/%s", name, name)
		}
	}
	for name := range loaded {
		if !fetched[name] {
			t.Errorf("shell.html loads /assets/%s but cache-assets.sh never fetches it", name)
		}
	}
}

// cachedAssets resolves the $ASSETS/js and $ASSETS/css paths cache-assets.sh
// declares, relative to $ASSETS, expanding the *_VERSION variables they name.
func cachedAssets(t *testing.T, script string) map[string]bool {
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

// An export cuts the shell's stylesheet and header fixation targets out of
// shell.html; pin that both are there and that the cut is the whole sheet.
func TestStandaloneAssets(t *testing.T) {
	a, err := StandaloneAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.ShellCSS, ".present-chart-note") ||
		strings.Contains(a.ShellCSS, "<style") ||
		strings.Contains(a.ShellCSS, "</style") {
		t.Error("ShellCSS is not the shell's inline stylesheet")
	}
	if !strings.Contains(a.FixationTargets, "[data-fixation]") {
		t.Errorf("FixationTargets = %q", a.FixationTargets)
	}
	if a.AppJS != string(appJS) || a.VizJS != string(vizJS) {
		t.Error("StandaloneAssets does not return the embedded app.js and viz.js")
	}
}
