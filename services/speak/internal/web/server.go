package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/mad01/thismoon/webkit"

	"github.com/mad01/thismoon/services/speak/internal/notify"
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

// enginezTimeout bounds the /enginez upstream ping; matches the read-aloud
// component's own probe timeout so both surfaces agree on "down".
const enginezTimeout = 1500 * time.Millisecond

// NewMux builds the speak HTTP handler: the markdown read-aloud page plus a
// CORS-enabled reverse proxy in front of the mlx-audio speech endpoint, so
// pages on other local origins (present.this etc.) can fetch speech from
// http://speak.this. version is the build sha baked in via ldflags, exposed
// at GET /version (the HTTP twin of the fleet-wide `speak version -o json`
// probe ralph uses for update detection).
func NewMux(ttsURL, version string) (*http.ServeMux, error) {
	upstream, err := url.Parse(ttsURL)
	if err != nil {
		return nil, fmt.Errorf("parse tts url %q: %w", ttsURL, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	// The engine sets its own CORS headers; ours are already on the response,
	// and duplicated Access-Control-Allow-Origin values make browsers reject
	// the response outright. Strip the upstream's set.
	proxy.ModifyResponse = func(res *http.Response) error {
		for _, h := range []string{
			"Access-Control-Allow-Origin", "Access-Control-Allow-Methods",
			"Access-Control-Allow-Headers", "Access-Control-Allow-Credentials",
			"Access-Control-Max-Age",
		} {
			res.Header.Del(h)
		}
		// The engine answered but rejected the request — record it. Success
		// responses are intentionally not emitted: read-aloud fans out one
		// request per sentence and would flood the event log.
		if res.StatusCode >= http.StatusBadRequest {
			notify.EmitEvent("speak", "error", "tts synthesis failed",
				fmt.Sprintf("upstream returned %s", res.Status),
				map[string]string{"status": fmt.Sprintf("%d", res.StatusCode)})
		}
		return nil
	}
	// The engine is unreachable (down, or the request never completed). Record
	// it, then fall back to the default 502 behaviour.
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		notify.EmitEvent("speak", "error", "tts engine unreachable", err.Error(), nil)
		w.WriteHeader(http.StatusBadGateway)
	}

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
		setCORS(w)
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
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		setCORS(w)
		w.WriteHeader(http.StatusNoContent)
	})

	// Engine reachability, distinct from /healthz: the component's own probe
	// hits GET / on its endpoint, which same-origin is this page — always up
	// even when the TTS engine behind the proxy is dead. app.js polls this to
	// warn that play buttons won't work. Any HTTP response from the upstream
	// (even 404) counts as reachable.
	mux.HandleFunc("GET /enginez", func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		ctx, cancel := context.WithTimeout(r.Context(), enginezTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.String()+"/", nil)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = res.Body.Close()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, "{\"version\":%q}\n", version)
	})

	return mux, nil
}

func setCORS(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Max-Age", "86400")
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

// Serve runs the HTTP server on 127.0.0.1:<port>. The wrapper handler adds the
// CORS header to every response so cross-origin probes and speech fetches work
// regardless of route.
func Serve(port int, ttsURL, version string) error {
	mux, err := NewMux(ttsURL, version)
	if err != nil {
		return err
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCORS(w)
		mux.ServeHTTP(w, r)
		log.Printf("%s %s", r.Method, r.URL.Path)
	})
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	log.Printf("speak: serving on http://%s (tts upstream %s)", addr, ttsURL)
	return http.ListenAndServe(addr, handler)
}
