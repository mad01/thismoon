// Package webkit ships the shared web chrome — palette, header, and the
// font/fixation/size/reload/theme controls — used by the mad01 local web tools
// (present, csl, catalog). The CSS/JS are authored in TypeScript under src/ and
// compiled to dist/ (committed), which is embedded here so consumers need no
// node toolchain. Mount Handler() at "GET /webkit/" to serve them.
package webkit

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// modulePath is the package import path, reported by GET /webkit/version.
const modulePath = "github.com/mad01/thismoon/webkit"

// assetHash is a short hex digest of the embedded dist bytes, computed once at
// package init. It changes whenever the compiled assets change — so it works as
// a real cache-busting ETag (the pinned pseudo-version did not, which let
// browsers hold stale CSS/JS until a force-refresh). It also doubles as the
// value returned by GET /webkit/version so the in-page poll can detect a deploy.
var assetHash = hashFS(distFS, "dist")

// hashFS walks fsys under root in a stable (lexical) file order and folds every
// file's path and contents into one SHA-256, returning a short hex prefix.
// fs.WalkDir yields entries in lexical order, so the digest is deterministic
// across builds for identical bytes and changes the moment any byte changes.
func hashFS(fsys fs.FS, root string) string {
	h := sha256.New()
	_ = fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Mix the path in too, so a file rename alone changes the hash.
		h.Write([]byte(path))
		h.Write([]byte{0})
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		h.Write(b)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Handler serves the embedded assets, stripping the /webkit/ prefix. Assets
// are served with no-cache + a content-hash ETag (assetHash), so browsers
// revalidate on every load (a cheap 304 on localhost) and pick up new asset
// bytes immediately — the ETag changes the moment dist/ changes. The previous
// ETag was the pinned pseudo-version, which did not move when the actual asset
// bytes did, so browsers held stale CSS/JS until a manual force-refresh.
// Mount it as: mux.Handle("GET /webkit/", webkit.Handler()).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.StripPrefix("/webkit/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Version metadata: confirm which webkit release a running tool serves.
		// Analogous to the ralph CLI `version -o json` convention. Not a
		// cacheable asset, so it bypasses the immutable file server.
		if r.URL.Path == "/webkit/version" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"module":  modulePath,
				"version": assetHash,
			})
			return
		}
		etag := `"` + assetHash + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		fileServer.ServeHTTP(w, r)
	})
}

// Mount wires the webkit assets onto mux at "GET /webkit/". It is the one-liner
// consumers should use instead of calling mux.Handle("GET /webkit/", Handler())
// by hand, so the route prefix stays consistent across tools.
func Mount(mux *http.ServeMux) {
	mux.Handle("GET /webkit/", Handler())
}

// BootScript returns the <script> tag that loads the FOUC-guard boot snippet
// served at /webkit/boot.js. It MUST be placed in <head>, before any paint and
// before webkit.js: the snippet reads the persisted theme + font size from
// localStorage and applies them on the document element synchronously, so the
// page never flashes the default theme. It is a classic blocking script (no
// defer/async) by design — that blocking is what prevents the flash. The same
// snippet is exported in JS as Webkit.bootSnippet for callers that prefer to
// inline it; both come from the single source src/boot.snippet.js.
func BootScript() template.HTML {
	return template.HTML(`<script src="/webkit/boot.js"></script>`)
}

// FS returns the embedded dist filesystem (webkit.css, webkit.js) for callers
// that prefer to read the assets directly rather than serve them over HTTP.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return distFS
	}
	return sub
}

// NoCacheHTML sets Cache-Control: no-cache on an HTML document response so the
// browser always revalidates before reusing the page. Without it, browsers
// heuristically cache documents that carry no caching headers, which makes the
// webkit version-poll soft-reload race a stale page from the back/forward cache.
// Consumers should call this when writing their own HTML so a webkit asset bump
// is reflected on the next navigation, not after a manual force-refresh.
func NoCacheHTML(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache")
}
