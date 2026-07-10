package blockpage

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandlerServesPageAndGames(t *testing.T) {
	h := Handler("")

	cases := []struct {
		path        string
		wantType    string
		wantContain string
	}{
		{"/", "text/html", "is blocked"},
		{"/some/blocked/path", "text/html", "is blocked"}, // catch-all still shows the page
		{"/missing.js", "text/html", "is blocked"},        // unknown scripts fall back to the page too
		{"/arcade.js", "application/javascript", "requestAnimationFrame"},
		{"/smash.js", "application/javascript", "ARCADE.register"},
		{"/gate.js", "application/javascript", "ARCADE.register"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := get(t, h, tc.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tc.wantType)
			}
			if body := rec.Body.String(); !strings.Contains(body, tc.wantContain) {
				t.Errorf("body for %q does not contain %q", tc.path, tc.wantContain)
			}
		})
	}
}

// Every script the page references must exist in the embedded assets, so a
// renamed or forgotten file fails here instead of 200-ing HTML at the browser.
func TestPageScriptsAreEmbedded(t *testing.T) {
	h := Handler("")
	page := string(indexHTML)
	for _, name := range []string{"arcade.js", "smash.js", "gate.js"} {
		if !strings.Contains(page, `src="/`+name+`"`) {
			t.Errorf("index.html does not reference %s", name)
		}
		rec := get(t, h, "/"+name)
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
			t.Errorf("%s served as %q, want application/javascript", name, ct)
		}
	}
}

func TestPluginGames(t *testing.T) {
	dir := t.TempDir()
	writeFile := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("rally.js", `ARCADE.register("rally", function (K) {});`)
	writeFile("zed-2.js", `ARCADE.register("zed-2", function (K) {});`)
	writeFile("Bad Name.js", `nope`)   // uppercase + space: not a plugin name
	writeFile("notes.txt", `not js`)   // wrong extension
	writeFile("smash.js", `shadowed?`) // collides with an embedded game
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(filepath.Join("sub", "deep.js"), `nested`)

	h := Handler(dir)

	t.Run("plugin scripts are served", func(t *testing.T) {
		for _, name := range []string{"rally.js", "zed-2.js"} {
			rec := get(t, h, "/"+name)
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
				t.Errorf("%s served as %q, want application/javascript", name, ct)
			}
			if !strings.Contains(rec.Body.String(), "ARCADE.register") {
				t.Errorf("%s body is not the plugin script", name)
			}
		}
	})

	t.Run("page lists plugin script tags after the embedded ones", func(t *testing.T) {
		body := get(t, h, "/").Body.String()
		for _, tag := range []string{`<script src="/rally.js"></script>`, `<script src="/zed-2.js"></script>`} {
			if !strings.Contains(body, tag) {
				t.Errorf("page missing %s", tag)
			}
		}
		if strings.Contains(body, "plugin-games") {
			t.Error("page still contains the plugin marker")
		}
		if strings.Index(body, "/arcade.js") > strings.Index(body, "/rally.js") {
			t.Error("kit script must load before plugin scripts")
		}
		for _, absent := range []string{"Bad Name.js", "notes.txt", "deep.js"} {
			if strings.Contains(body, absent) {
				t.Errorf("page lists non-plugin file %s", absent)
			}
		}
	})

	t.Run("embedded game shadows a plugin with the same name", func(t *testing.T) {
		body := get(t, h, "/smash.js").Body.String()
		if !strings.Contains(body, `ARCADE.register("smash"`) {
			t.Error("embedded smash.js was shadowed by the plugin file")
		}
	})

	t.Run("invalid names fall back to the page", func(t *testing.T) {
		for _, path := range []string{"/notes.txt", "/sub/deep.js", "/../rally.js", "/Bad%20Name.js"} {
			rec := get(t, h, path)
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("%s served as %q, want the block page", path, ct)
			}
		}
	})

	t.Run("missing dir means embedded only", func(t *testing.T) {
		h := Handler(filepath.Join(dir, "does-not-exist"))
		body := get(t, h, "/").Body.String()
		if strings.Contains(body, "rally.js") || strings.Contains(body, "plugin-games") {
			t.Error("page should list only embedded games")
		}
	})
}
