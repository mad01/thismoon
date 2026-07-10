// Package proxy is the host-based reverse proxy: one front door on
// 127.0.0.1:80 that routes each request to a backend port by its Host header.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/d-man/internal/blockpage"
	"github.com/mad01/thismoon/services/d-man/internal/config"
)

type ctxKey int

const origHostKey ctxKey = 0

// SitesPath is a reserved path d-man answers itself (instead of proxying) on any
// configured host: it returns the navigable site list as JSON for the ⌘K picker.
// Because it resolves on whatever ".this" host the page is on, the picker's fetch
// is same-origin.
const SitesPath = "/__this/sites.json"

// probeTimeout caps a single backend liveness probe. Backends are on loopback,
// so a live one answers in microseconds; this only bounds the wait on a dead
// port whose connection never completes.
const probeTimeout = 800 * time.Millisecond

// sitesTTL is how long a probed site list is cached before the next fetch
// re-probes. routes.toml lists every *possible* site; the picker should show
// only the ones actually running on this host, and this bounds how stale that
// liveness view can be while keeping probe traffic off the hot path.
const sitesTTL = 30 * time.Second

// Prober reports whether a backend at "host:port" is currently serving.
type Prober func(backend string) bool

// Handler routes by Host header using a host -> backend map. It is safe to
// build a fresh Handler and swap it in on config reload.
type Handler struct {
	routes  map[string]*url.URL
	rp      *httputil.ReverseProxy
	blocked map[string]bool // hosts served the block page instead of proxied
	block   http.Handler    // renders the block-page minigame

	sites []config.Site // all port-backed sites; filtered by liveness on serve
	probe Prober        // injectable for tests; defaults to an HTTP loopback probe
	ttl   time.Duration

	cacheMu   sync.Mutex
	cacheJSON []byte    // last probed+marshaled body served at SitesPath
	cacheAt   time.Time // when cacheJSON was built
}

// New builds a Handler from a host -> "backendHost:port" map (config.RouteMap).
// sites is the full set of navigable sites; the SitesPath body is filtered to
// the ones whose backend currently responds, re-probed at most every sitesTTL.
// blocked hosts are served the local block page instead of being proxied;
// gamesDir optionally names a directory of plugin block-page games.
func New(routeMap map[string]string, sites []config.Site, blocked []string, gamesDir string) (*Handler, error) {
	routes := make(map[string]*url.URL, len(routeMap))
	for host, backend := range routeMap {
		target, err := url.Parse("http://" + backend)
		if err != nil {
			return nil, fmt.Errorf("route %s: bad backend %q: %w", host, backend, err)
		}
		routes[normalizeHost(host)] = target
	}
	blockSet := make(map[string]bool, len(blocked))
	for _, host := range blocked {
		blockSet[normalizeHost(host)] = true
	}
	h := &Handler{
		routes:  routes,
		blocked: blockSet,
		block:   blockpage.Handler(gamesDir),
		sites:   sites,
		probe:   httpProbe(probeTimeout),
		ttl:     sitesTTL,
	}
	h.rp = &httputil.ReverseProxy{
		// Rewrite picks the backend by the inbound Host and stashes the original
		// host so ModifyResponse can undo backend self-redirects.
		Rewrite: func(pr *httputil.ProxyRequest) {
			target := h.routes[normalizeHost(pr.In.Host)]
			pr.SetURL(target)
			pr.SetXForwarded()
			pr.Out = pr.Out.WithContext(
				context.WithValue(pr.Out.Context(), origHostKey, pr.In.Host),
			)
		},
		// ModifyResponse rewrites a Location that points at the backend host
		// back to the hostname the client used, so a redirect (e.g. a backend
		// adding a trailing slash) keeps the user on http://<name>.this/ instead
		// of bouncing them to http://127.0.0.1:<port>/.
		ModifyResponse: func(resp *http.Response) error {
			loc := resp.Header.Get("Location")
			if loc == "" {
				return nil
			}
			u, err := url.Parse(loc)
			if err != nil || u.Host == "" {
				return nil // relative redirect: nothing to rewrite
			}
			if u.Host == resp.Request.URL.Host {
				if orig, ok := resp.Request.Context().Value(origHostKey).(string); ok &&
					orig != "" {
					u.Host = orig
					u.Scheme = "http"
					resp.Header.Set("Location", u.String())
				}
			}
			return nil
		},
	}
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// d-man owns SitesPath on every host — serve the site list instead of
	// proxying. Same-origin from the page's perspective; CORS lets a page not
	// behind d-man (localhost dev) use it as a cross-origin fallback.
	if r.URL.Path == SitesPath {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(h.sitesBody())
		return
	}

	host := normalizeHost(r.Host)
	// Blocked hosts never reach a backend — d-man owns them and serves the
	// block-page minigame on every path.
	if h.blocked[host] {
		h.block.ServeHTTP(w, r)
		return
	}
	if _, ok := h.routes[host]; !ok {
		http.Error(w,
			fmt.Sprintf("d-man: no route for %q (add it to routes.toml)", host),
			http.StatusBadGateway)
		return
	}
	h.rp.ServeHTTP(w, r)
}

// sitesBody returns the JSON body for SitesPath: the configured sites filtered
// to those whose backend currently responds. The result is cached for ttl, and
// the lock makes a single goroutine re-probe while others wait for and share
// its result — so a burst of picker fetches triggers one probe round, not one
// per request.
func (h *Handler) sitesBody() []byte {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.cacheJSON != nil && time.Since(h.cacheAt) < h.ttl {
		return h.cacheJSON
	}
	body, err := json.Marshal(h.liveSites())
	if err != nil || len(body) == 0 {
		body = []byte("[]")
	}
	h.cacheJSON = body
	h.cacheAt = time.Now()
	return h.cacheJSON
}

// liveSites probes every site's backend concurrently and returns the ones that
// respond, preserving config order. A site with no backend or a dead one is
// dropped, so the picker only ever lists sites actually running on this host.
func (h *Handler) liveSites() []config.Site {
	up := make([]bool, len(h.sites))
	var wg sync.WaitGroup
	for i, s := range h.sites {
		if s.Backend == "" {
			continue
		}
		wg.Add(1)
		go func(i int, backend string) {
			defer wg.Done()
			up[i] = h.probe(backend)
		}(i, s.Backend)
	}
	wg.Wait()

	live := make([]config.Site, 0, len(h.sites))
	for i, s := range h.sites {
		if up[i] {
			live = append(live, s)
		}
	}
	return live
}

// httpProbe returns a Prober that counts a backend as up when GET / yields any
// response below 500 — a 404 from a service with no root route still proves the
// process is listening (same liveness rule the status page uses).
func httpProbe(timeout time.Duration) Prober {
	client := &http.Client{Timeout: timeout}
	return func(backend string) bool {
		resp, err := client.Get("http://" + backend + "/")
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode < 500
	}
}

// Hosts returns the configured front-door hostnames (for logging / debug).
func (h *Handler) Hosts() []string {
	hosts := make([]string, 0, len(h.routes))
	for host := range h.routes {
		hosts = append(hosts, host)
	}
	return hosts
}

// normalizeHost canonicalizes a Host header for lookup: strip an optional
// :port, a trailing FQDN dot ("present.this." -> "present.this"), and case.
// DNS names are case-insensitive and browsers occasionally send the
// fully-qualified trailing-dot form, so both must match the same route.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}
