// Package blockpage serves the local "you're blocked" page for hosts on
// d-man's block list: a self-contained retro arcade bundled into the binary
// via go:embed, one game picked at random per visit. Extra plugin games load
// from an optional local directory (games_dir in routes.toml) — any .js file
// there that calls ARCADE.register joins the rotation. A blocked host is
// handed entirely to this handler instead of being proxied, so every path on
// it shows the game.
package blockpage

import (
	"bytes"
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed assets
var assetsFS embed.FS

var indexHTML = func() []byte {
	b, err := assetsFS.ReadFile("assets/index.html")
	if err != nil {
		// Embedded at build time; a read miss is a build bug, not a runtime one.
		panic("blockpage: missing embedded asset index.html: " + err.Error())
	}
	return b
}()

// pluginMarker is where renderIndex splices <script> tags for plugin games,
// after the embedded ones so the kit is always loaded first.
const pluginMarker = "<!-- plugin-games -->"

// pluginName is the only shape a plugin script name may have: a flat,
// lowercase .js basename. Anything else falls through to the block page.
var pluginName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*\.js$`)

// Handler returns an http.Handler that serves the block page and its game
// scripts. It owns all paths on a blocked host: a .js path matching an
// embedded asset or a file in gamesDir is a game script, everything else
// renders the page. gamesDir may be empty (embedded games only); it is read
// per request, so dropping a script in hot-loads it with no restart.
func Handler(gamesDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if name, ok := scriptName(r.URL.Path); ok {
			// Embedded first: a plugin can never shadow the kit or a bundled game.
			if b, err := assetsFS.ReadFile("assets/" + name); err == nil {
				write(w, "application/javascript; charset=utf-8", b)
				return
			}
			if b, ok := pluginScript(gamesDir, name); ok {
				write(w, "application/javascript; charset=utf-8", b)
				return
			}
		}
		write(w, "text/html; charset=utf-8", renderIndex(gamesDir))
	})
	return mux
}

// scriptName extracts a servable script basename from a request path:
// "/smash.js" -> "smash.js". Paths with directories never name a script.
func scriptName(path string) (string, bool) {
	name := strings.TrimPrefix(path, "/")
	if strings.Contains(name, "/") || !pluginName.MatchString(name) {
		return "", false
	}
	return name, true
}

// pluginScript reads one plugin game from gamesDir. The name is already
// validated flat by scriptName, so it cannot escape the directory.
func pluginScript(gamesDir, name string) ([]byte, bool) {
	if gamesDir == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(gamesDir, name))
	if err != nil {
		return nil, false
	}
	return b, true
}

// renderIndex replaces the plugin marker in the embedded page with <script>
// tags for the games currently in gamesDir (sorted, so load order is stable).
func renderIndex(gamesDir string) []byte {
	names := pluginGames(gamesDir)
	if len(names) == 0 {
		return bytes.Replace(indexHTML, []byte(pluginMarker), nil, 1)
	}
	var tags strings.Builder
	for _, n := range names {
		tags.WriteString(`<script src="/`)
		tags.WriteString(n)
		tags.WriteString("\"></script>\n")
	}
	return bytes.Replace(indexHTML, []byte(pluginMarker), []byte(tags.String()), 1)
}

// pluginGames lists the plugin script names in gamesDir. A missing or
// unreadable directory means no plugins, never an error — the dir may simply
// not exist yet on this machine.
func pluginGames(gamesDir string) []string {
	if gamesDir == "" {
		return nil
	}
	entries, err := os.ReadDir(gamesDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !pluginName.MatchString(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func write(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
