package buildinfo

import (
	"io"
	"net/http"
)

// Handler serves the linked-in build metadata. Wire it as
// mux.HandleFunc("GET /version", buildinfo.Handler()).
func Handler() http.HandlerFunc {
	return Get().Handler()
}

// Handler returns a GET /version handler serving i. A server handed an Info at
// construction (so tests can pin one) uses this; the package-level Handler is
// the shorthand for the linked-in metadata. The body is rendered once, at wiring
// time — build metadata never changes while the process runs.
func (i Info) Handler() http.HandlerFunc {
	body := i.PrettyJSON()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, body)
	}
}
