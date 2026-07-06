package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed assets
var assetsFS embed.FS

// pageBytes returns the bytes of an embedded HTML page (e.g. "index.html").
func pageBytes(name string) ([]byte, error) {
	b, err := assetsFS.ReadFile("assets/" + name)
	if err != nil {
		return nil, fmt.Errorf("missing embedded page %q: %w", name, err)
	}
	return b, nil
}

// assetsHandler serves static files under assets/static at /assets/* with long
// cache headers.
func assetsHandler() http.Handler {
	sub, err := fs.Sub(assetsFS, "assets/static")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		fileServer.ServeHTTP(w, r)
	})
}
