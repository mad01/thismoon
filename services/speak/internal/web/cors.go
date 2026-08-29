package web

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// thisSuffix is the hostname suffix the local domain front door serves. A
// page on http://<name>.this is one of this machine's own surfaces, so it
// belongs on the allowlist beside loopback.
const thisSuffix = ".this"

// setCORS reflects an allowed request origin back to the browser and sends
// no CORS headers at all for anything else. Reflection rather than "*" is
// the point: the speech endpoint drives this machine's TTS engine, and "*"
// let any page the user happened to be browsing fetch from it. The origins
// that legitimately fetch speech are the sibling .this pages (present, csl)
// and localhost dev servers, all of which the allowlist keeps working.
func setCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !originAllowed(origin) {
		return
	}
	h := w.Header()
	// The response varies by origin, so a cache must key on the header.
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Max-Age", "86400")
}

// originAllowed reports whether origin names a page on this machine: an
// http or https URL whose host is loopback (localhost, 127.0.0.0/8, ::1, on
// any port) or ends in .this. An absent, opaque ("null"), or unparseable
// Origin is never allowed.
func originAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, thisSuffix) {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
