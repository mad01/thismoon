package server

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"net/http"
)

// logoPNG is the repo logo the deck view shows in a corner of every deck
// unless the deck's chrome says otherwise. docs/assets/logo.png is the
// source; this is a byte-identical copy beside the code, because go:embed
// cannot reach outside the package directory, and TestEmbeddedLogoMatchesDocs
// keeps the two in step the way the k8s store keeps its CRD and
// deploy/base/crd.yaml together. It is served from the binary at
// GET /logo.png rather than from the workdir assets directory, so a shared
// instance needs no new file and no config.
//
//go:embed logo.png
var logoPNG []byte

// logoETag is the content hash the logo route answers with. The route sends
// no-cache plus this ETag, as webkit's assets do, instead of the one-year
// immutable header the workdir assets get, so a later logo change is not
// pinned in browsers.
var logoETag = func() string {
	sum := sha256.Sum256(logoPNG)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}()

// handleLogo serves the embedded logo.
func handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("ETag", logoETag)
	w.Header().Set("Cache-Control", "no-cache")
	if r.Header.Get("If-None-Match") == logoETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(logoPNG)
}
