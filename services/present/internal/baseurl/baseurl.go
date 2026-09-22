// Package baseurl derives the public base URL a shared instance is reached
// through. Nothing configures the hostname: the ingress or service mesh in
// front sets X-Forwarded-Host and X-Forwarded-Proto, and Middleware fills
// them from the request itself when nothing did. One header-only function
// then serves HTTP handlers and MCP tool handlers alike; a tool handler sees
// only the header map, and Go moves Host out of it into Request.Host.
package baseurl

import (
	"net/http"
	"strings"
)

const (
	hostHeader  = "X-Forwarded-Host"
	protoHeader = "X-Forwarded-Proto"
)

// Middleware sets X-Forwarded-Host from the request's Host and
// X-Forwarded-Proto from whether TLS terminated here, each only when the
// header is absent, then calls next.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(hostHeader) == "" && r.Host != "" {
			r.Header.Set(hostHeader, r.Host)
		}
		if r.Header.Get(protoHeader) == "" {
			proto := "http"
			if r.TLS != nil {
				proto = "https"
			}
			r.Header.Set(protoHeader, proto)
		}
		next.ServeHTTP(w, r)
	})
}

// FromHeader returns scheme://host built from the forwarded headers, with
// no trailing slash. override, when set, wins outright (it is the
// --base-url display setting). A proxy chain may append values with commas;
// the first one is the edge the client spoke to. Without a forwarded host
// the result is empty, and callers fall back to whatever they have.
func FromHeader(h http.Header, override string) string {
	if override != "" {
		return strings.TrimRight(override, "/")
	}
	if h == nil {
		return ""
	}
	host := first(h.Get(hostHeader))
	if host == "" {
		return ""
	}
	proto := first(h.Get(protoHeader))
	if proto == "" {
		proto = "http"
	}
	return proto + "://" + host
}

func first(v string) string {
	v, _, _ = strings.Cut(v, ",")
	return strings.TrimSpace(v)
}
