// Package blockpage serves the local "you're blocked" page for hosts on
// d-man's block list: a self-contained, dependency-free 2D endless-runner
// minigame (Chrome-dino style) bundled into the binary via go:embed. A blocked
// host is handed entirely to this handler instead of being proxied, so every
// path on it shows the game.
package blockpage

import (
	"embed"
	"net/http"
)

//go:embed assets
var assetsFS embed.FS

func mustAsset(name string) []byte {
	b, err := assetsFS.ReadFile("assets/" + name)
	if err != nil {
		// Embedded at build time; a read miss is a build bug, not a runtime one.
		panic("blockpage: missing embedded asset " + name + ": " + err.Error())
	}
	return b
}

var (
	indexHTML = mustAsset("index.html")
	dinoJS    = mustAsset("dino.js")
)

// Handler returns an http.Handler that serves the block page and its game
// script. It owns all paths on a blocked host: /dino.js is the game, everything
// else renders the page.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/dino.js", func(w http.ResponseWriter, _ *http.Request) {
		write(w, "application/javascript; charset=utf-8", dinoJS)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		write(w, "text/html; charset=utf-8", indexHTML)
	})
	return mux
}

func write(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
