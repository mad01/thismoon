package server

import (
	"bufio"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
)

// Keys is the set of keys a shared instance accepts, held as the hex
// SHA-256 of each key: the instance never holds a key itself.
type Keys struct {
	hashes [][]byte
}

// emptyKeyHash is the SHA-256 of the empty string, which no request may
// pass as a key.
const emptyKeyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// ParseKeys reads a keys file: one lowercase hex SHA-256 of a key per line.
// Blank lines and lines that start with # are skipped. A line that is not
// a hash, the hash of the empty key, or a file without a key is an error.
func ParseKeys(r io.Reader) (Keys, error) {
	var k Keys
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		raw, err := hex.DecodeString(line)
		if err != nil || len(raw) != sha256.Size || line != strings.ToLower(line) {
			return Keys{}, fmt.Errorf("line %d: want the lowercase hex SHA-256 of a key", n)
		}
		if line == emptyKeyHash {
			return Keys{}, fmt.Errorf("line %d: the hash of an empty key", n)
		}
		k.hashes = append(k.hashes, []byte(line))
	}
	if err := sc.Err(); err != nil {
		return Keys{}, err
	}
	if len(k.hashes) == 0 {
		return Keys{}, errors.New("no key hash in the keys file")
	}
	return k, nil
}

// Allows reports whether key hashes to an accepted hash. Every hash is
// compared in constant time, so the answer takes as long whichever matches.
func (k Keys) Allows(key string) bool {
	if key == "" {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	got := []byte(hex.EncodeToString(sum[:]))
	ok := 0
	for _, want := range k.hashes {
		ok |= subtle.ConstantTimeCompare(got, want)
	}
	return ok == 1
}

// SharedConfig is what a shared instance adds to the routes Handler serves.
type SharedConfig struct {
	// Keys are the keys every request but the page shell's needs.
	Keys Keys
	// MCP serves the MCP tools at /mcp; nil leaves the route out.
	MCP http.Handler
}

// SharedHandler serves a shared instance: the page and the read API of
// Handler, POST /api/sync and /api/sync/missing for the replicate loops of
// local serves, and the MCP tools at /mcp. Every route needs a key (see
// requireKey) but the ones that hold no record: the page shell, its
// script, the webkit assets, /healthz and /version.
func (s *Server) SharedHandler(cfg SharedConfig) http.Handler {
	mux := s.routes()
	mux.HandleFunc("POST /api/sync", s.handleSync)
	mux.HandleFunc("POST /api/sync/missing", s.handleSyncMissing)
	if cfg.MCP != nil {
		mux.Handle("/mcp", cfg.MCP)
	}
	return logRequests(requireKey(cfg.Keys, mux))
}

// APIHandler is the read API and the page with no request log and no key,
// for the MCP tools of a shared instance to call in process once /mcp has
// checked the caller's key.
func (s *Server) APIHandler() http.Handler { return s.routes() }

// requireKey passes a request on when it needs no key or carries an
// accepted one: a bearer token, or on a GET under /api/ the cookie the
// page keeps its key in. Writes and /mcp take the bearer token only, so a
// page on another site cannot use the cookie to send records. Every answer
// varies by both, so no cache hands one caller's records to another.
func requireKey(keys Keys, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Authorization, Cookie")
		if keyFree(r) || keys.Allows(requestKey(r)) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="moondb"`)
		writeErr(w, http.StatusUnauthorized,
			errors.New("moondb: this instance needs a key: send it as a bearer token"))
	})
}

// keyFree reports whether r asks for something that holds no record.
func keyFree(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	switch r.URL.Path {
	case "/", "/app.js", "/healthz", "/version":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/webkit/")
}

// requestKey is the key r carries: its bearer token, else on a GET under
// /api/ the page's key cookie, which the page stores URI-encoded, else "".
func requestKey(r *http.Request) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(token)
	}
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/") {
		return ""
	}
	c, err := r.Cookie(moondb.KeyCookie)
	if err != nil {
		return ""
	}
	key, err := url.PathUnescape(c.Value)
	if err != nil {
		return ""
	}
	return key
}
