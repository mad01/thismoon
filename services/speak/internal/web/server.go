package web

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/speak/internal/audiocache"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// indexHTML is the landing page served at GET /: static, naming the service,
// pointing at present (where documents are read) and listing the routes. The
// engine state on it is fetched client-side from GET /enginez, so serving the
// page never runs a synthesis.
//
//go:embed assets/index.html
var indexHTML []byte

const maxReadBytes = 5 << 20 // 5MB of block text is plenty for one page

// cacheTTL is how long a synthesized clip stays on disk unused: a document
// reread within a month plays without waiting for the provider again.
const cacheTTL = 30 * 24 * time.Hour

// maxCacheBytes caps the audio cache: about 12 hours of 24 kHz mono WAV
// (~170 MB an hour), since live speech is cached along with prepared
// documents. Past it, the least recently used clips go first.
const maxCacheBytes = 2 << 30

// reapInterval is how often serve removes clips unused for cacheTTL or past
// maxCacheBytes.
const reapInterval = 24 * time.Hour

// Config is what speak serve serves from.
type Config struct {
	// Speaker is the active provider.
	Speaker Speaker
	// Health is the one state every synthesis and /enginez probe records
	// into, so the engine line and the error bodies agree.
	Health *tts.Health
	// Info is the build metadata linked in via ldflags, exposed at GET
	// /version (the HTTP twin of the fleet-wide `speak version -o json`
	// probe ralph uses for update detection).
	Info buildinfo.Info
	// CacheDir holds synthesized clips; required.
	CacheDir string

	// retryDelay overrides the preparer's backoff between attempts, so
	// tests do not wait it out; nil in production.
	retryDelay func(attempt int) time.Duration
}

// NewMux builds the speak HTTP handler: the document audio routes a page
// registers its text with and plays prepared parts from, an OpenAI-style
// speech endpoint that other local origins (present.this etc.) fetch speech
// from, both subject to the CORS allowlist in cors.go, and the static landing
// page. The audio routes answer from the cache in cfg.CacheDir when they can.
func NewMux(cfg Config) *http.ServeMux {
	store := audiocache.NewStore(cfg.CacheDir)
	speech := &speechHandler{speaker: cfg.Speaker, health: cfg.Health, store: store}

	mux := http.NewServeMux()
	webkit.Mount(mux)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// no-store: the page is small, probed often (status, every present
		// page view) and a rebuild's copy must show on the next load.
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexHTML)
	})

	newDocServer(cfg, store).routes(mux)

	// CORS here too, so the mux answers a sibling page's fetch on its own;
	// the preflight before it is handler's, like every route's.
	mux.HandleFunc("POST /v1/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		speech.ServeHTTP(w, r)
	})

	// CORS on the root probe too: <wk-read-aloud> checks GET / cross-origin
	// before injecting any buttons.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.WriteHeader(http.StatusNoContent)
	})

	// Engine health, distinct from /healthz: the component's own probe hits
	// GET / on its endpoint, which is always up even when the TTS engine
	// behind it is dead. The landing page fetches this client-side to show
	// the engine state; see enginez in speech.go.
	mux.Handle("GET /enginez", &enginez{speaker: cfg.Speaker, health: cfg.Health})

	mux.HandleFunc("GET /version", cfg.Info.Handler())

	return mux
}

// Serve runs the HTTP server on 127.0.0.1:<port> (see handler for what
// wraps the routes). Clips unused for cacheTTL, then the least recently
// used past maxCacheBytes, are removed at start and every reapInterval.
func Serve(port int, cfg Config) error {
	if cfg.CacheDir == "" {
		return errors.New("speak: serve needs an audio cache directory")
	}
	stop := make(chan struct{})
	defer close(stop)
	go reapCache(audiocache.NewStore(cfg.CacheDir), stop)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	state := cfg.Health.Snapshot()
	log.Printf(
		"speak: serving on http://%s (tts provider %s, model %s, audio cache %s)",
		addr,
		state.Provider,
		state.Model,
		cfg.CacheDir,
	)
	return http.ListenAndServe(addr, handler(cfg))
}

// handler is NewMux behind what every request passes first: the CORS
// allowlist on every response, so cross-origin probes and speech fetches
// from this machine's own pages work on any route; the cross-site guard; and
// the preflight answer, so a page on the allowlist can post JSON to any route
// without each route handling OPTIONS. A preflight from an allowed origin
// gets 204 carrying the permission setCORS put on; one from anywhere else is
// refused like any other request.
func handler(cfg Config) http.Handler {
	mux := NewMux(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		if reason := crossSite(r); reason != "" {
			log.Printf("%s %s refused: %s", r.Method, r.URL.Path, reason)
			writeError(w, http.StatusForbidden, "speak serves only this machine's own pages: "+
				reason)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
		} else {
			mux.ServeHTTP(w, r)
		}
		log.Printf("%s %s", r.Method, r.URL.Path)
	})
}

// crossSite explains why r comes from a page that is not this machine's
// own, or returns "" when it does not. Leaving out CORS headers is not
// enough: a browser still sends a simple request (a form post, a text/plain
// fetch, an <audio src>) and only hides the answer, and these routes spend
// synthesis before they answer. A page that is not on the allowlist gives
// itself away by its Origin, or, where a browser sends none (media, image
// and frame loads, links), by Sec-Fetch-Site. Requests with neither (curl,
// speak doctor) pass, and so does a link to the page itself from anywhere;
// a link or frame pointing at an audio route does not.
func crossSite(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		if originAllowed(origin) {
			return ""
		}
		return fmt.Sprintf("origin %q is not on the allowlist", origin)
	}
	if r.Header.Get("Sec-Fetch-Site") != "cross-site" || isPageNavigation(r) {
		return ""
	}
	return "a cross-site request from another page"
}

// isPageNavigation reports a browser opening the page itself, GET /, in a
// tab: the one cross-site navigation speak takes.
func isPageNavigation(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/" &&
		r.Header.Get("Sec-Fetch-Mode") == "navigate" &&
		r.Header.Get("Sec-Fetch-Dest") == "document"
}

// reapCache removes clips unused for cacheTTL, then the least recently used
// past maxCacheBytes, now and every reapInterval until stop closes.
func reapCache(store *audiocache.Store, stop <-chan struct{}) {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()
	for {
		reaped, err := store.Reap(cacheTTL, maxCacheBytes, time.Now())
		if err != nil {
			log.Printf("speak: reap audio cache: %v", err)
		}
		if reaped.Expired+reaped.Evicted > 0 {
			log.Printf("speak: audio cache: removed %d clips unused for %d days and %d "+
				"least recently used past the %d GiB cap", reaped.Expired,
				int(cacheTTL.Hours()/24), reaped.Evicted, maxCacheBytes>>30)
		}
		select {
		case <-ticker.C:
		case <-stop:
			return
		}
	}
}
