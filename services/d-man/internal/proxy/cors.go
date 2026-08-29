package proxy

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// thisSuffix is the hostname suffix d-man serves. Every page behind the front
// door is reachable as http://<name>.this, so that suffix plus loopback is
// the whole set of origins this machine's own pages can have.
const thisSuffix = ".this"

// setSitesCORS reflects an allowed request origin back on the site-list
// response and sends no CORS headers for anything else. The endpoint answers
// on every configured host, so pages behind d-man reach it same-origin and
// need no header at all; the allowlist exists for the localhost-port case (a
// page opened at 127.0.0.1:<port> instead of through the front door). "*"
// would additionally hand the list of this machine's local services to any
// page the user happens to be browsing.
func setSitesCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if !originAllowed(origin) {
		return
	}
	// The response varies by origin, so a cache must key on the header.
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Origin", origin)
}

// originAllowed reports whether origin names a page on this machine: an http
// or https URL whose host is loopback (localhost, 127.0.0.0/8, ::1, on any
// port) or ends in .this. An absent, opaque ("null"), or unparseable Origin
// is never allowed.
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
