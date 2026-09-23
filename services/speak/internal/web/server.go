package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/buildinfo"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/tts"
	"github.com/mad01/thismoon/services/speak/internal/ttsclient"
)

// shellHTML is the static page shell (chrome only). The page body — header,
// upload form, and rendered doc — is built client-side by appJS, which posts
// uploads to /read and mounts the returned sections. appJS is served at
// GET /app.js.
//
//go:embed assets/shell.html
var shellHTML []byte

//go:embed assets/app.js
var appJS []byte

const maxUploadBytes = 5 << 20 // 5MB markdown is plenty for a local tool

// NewMux builds the speak HTTP handler: the markdown read-aloud page plus a
// reverse proxy in front of the mlx-audio speech endpoint that other local
// origins (present.this etc.) can fetch speech from, subject to the CORS
// allowlist in cors.go. info is the build metadata linked in via ldflags, exposed
// at GET /version (the HTTP twin of the fleet-wide `speak version -o json`
// probe ralph uses for update detection).
func NewMux(ttsURL string, info buildinfo.Info) (*http.ServeMux, error) {
	upstream, err := url.Parse(ttsURL)
	if err != nil {
		return nil, fmt.Errorf("parse tts url %q: %w", ttsURL, err)
	}
	// One health state for the process: every proxied request and every
	// /enginez probe records into it, so the banner and the error bodies agree.
	health := tts.NewHealth(ttsclient.Provider, speak.DefaultModel)
	proxy := newSpeechProxy(upstream, health)

	mux := http.NewServeMux()
	webkit.Mount(mux)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(shellHTML)
	})

	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		// no-cache so a speak rebuild's app.js is picked up on the next load.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(appJS)
	})

	mux.HandleFunc("POST /read", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		file, header, err := r.FormFile("doc")
		if err != nil {
			http.Error(
				w,
				"upload a markdown file in the 'doc' field: "+err.Error(),
				http.StatusBadRequest,
			)
			return
		}
		defer func() { _ = file.Close() }()
		source, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read upload: "+err.Error(), http.StatusBadRequest)
			return
		}
		content, err := RenderSections(source)
		if err != nil {
			http.Error(w, "render markdown: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		writeReadJSON(w, header.Filename, content)
	})

	mux.HandleFunc("/v1/audio/speech", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		proxy.ServeHTTP(w, r)
	})

	// CORS on the root probe too: <wk-read-aloud> checks GET / cross-origin
	// before injecting any buttons.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		w.WriteHeader(http.StatusNoContent)
	})

	// Engine health, distinct from /healthz: the component's own probe hits
	// GET / on its endpoint, which same-origin is this page — always up even
	// when the TTS engine behind the proxy is dead. app.js polls this to warn
	// that play buttons won't work and why; see enginez in speech.go.
	mux.Handle("GET /enginez", &enginez{engine: ttsclient.New(ttsURL), health: health})

	mux.HandleFunc("GET /version", info.Handler())

	return mux, nil
}

// readResponse is the JSON shape POST /read returns: the uploaded file name and
// the rendered HTML body (goldmark sections). app.js mounts Content as-is and
// shows Name as the doc label — the data the old {{DOC_NAME}}/{{CONTENT}}
// template substitution baked into a full page, now delivered for client render.
type readResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func writeReadJSON(w http.ResponseWriter, docName, content string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(readResponse{Name: docName, Content: content}); err != nil {
		log.Printf("speak: encode read response: %v", err)
	}
}

// Serve runs the HTTP server on 127.0.0.1:<port>. The wrapper handler applies
// the CORS allowlist to every response so cross-origin probes and speech
// fetches from this machine's own pages work regardless of route.
func Serve(port int, ttsURL string, info buildinfo.Info) error {
	mux, err := NewMux(ttsURL, info)
	if err != nil {
		return err
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORS(w, r)
		mux.ServeHTTP(w, r)
		log.Printf("%s %s", r.Method, r.URL.Path)
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("speak: serving on http://%s (tts upstream %s)", addr, ttsURL)
	return http.ListenAndServe(addr, handler)
}
