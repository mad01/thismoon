// Package config loads and validates the d-man routes file. A route maps a
// short name (under a default suffix) to a local backend port; the resolved
// host is what both /etc/hosts and the reverse proxy key on.
package config

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultSuffix is the top-level label appended to route names when the config
// does not set one. "csl" + "this" => "csl.this".
const DefaultSuffix = "this"

// DefaultTarget is the backend host a route proxies to when none is given. All
// services live on loopback; only the port differs.
const DefaultTarget = "127.0.0.1"

// Config is the parsed routes file.
type Config struct {
	Suffix string  `toml:"suffix"`
	Routes []Route `toml:"route"`
}

// Route maps one name to one backend. A route is either port-backed (Port set)
// or a CNAME alias (Cname set) pointing at another route's name — never both.
type Route struct {
	Name   string `toml:"name"`   // short label, e.g. "csl" -> csl.<suffix>
	Port   int    `toml:"port"`   // backend port on Target (port-backed route)
	Target string `toml:"target"` // backend host, defaults to 127.0.0.1
	Cname  string `toml:"cname"`  // alias: name of another route to resolve to
}

func (r Route) isCname() bool { return r.Cname != "" }

// hostLabel matches a single DNS label per RFC 1123: alphanumeric with internal
// hyphens, 1-63 chars. Route names may also be dotted (a.b), so Host validation
// checks each label.
var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// Load reads, parses, defaults, and validates the config at path.
func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Suffix == "" {
		c.Suffix = DefaultSuffix
	}
	for i := range c.Routes {
		// CNAME aliases inherit their backend's target; only port-backed routes
		// carry a target of their own.
		if !c.Routes[i].isCname() && c.Routes[i].Target == "" {
			c.Routes[i].Target = DefaultTarget
		}
	}
}

// Validate rejects configs that would produce an illegal hostname, an
// unroutable backend, or an unresolvable CNAME. It fails fast before anything
// touches /etc/hosts.
func (c *Config) Validate() error {
	if !validHostname(c.Suffix) {
		return fmt.Errorf("invalid suffix %q", c.Suffix)
	}
	names := make(map[string]bool, len(c.Routes))
	seenHost := make(map[string]bool, len(c.Routes))
	for _, r := range c.Routes {
		if r.Name == "" {
			return fmt.Errorf("route with empty name")
		}
		if !validHostname(r.Name) {
			return fmt.Errorf("invalid route name %q", r.Name)
		}
		if names[r.Name] {
			return fmt.Errorf("duplicate route name %q", r.Name)
		}
		names[r.Name] = true

		host := c.hostFor(r)
		if !validHostname(host) {
			return fmt.Errorf("route %q produces invalid host %q", r.Name, host)
		}
		if seenHost[host] {
			return fmt.Errorf("duplicate host %q", host)
		}
		seenHost[host] = true
	}

	// Second pass: now that all names are known, validate backends and resolve
	// CNAME chains (needs the full name set).
	for _, r := range c.Routes {
		switch {
		case r.isCname() && r.Port != 0:
			return fmt.Errorf("route %q: set either port or cname, not both", r.Name)
		case r.isCname():
			if _, err := c.resolve(r.Name); err != nil {
				return err
			}
		case r.Port != 0:
			if r.Port < 1 || r.Port > 65535 {
				return fmt.Errorf("route %q: port %d out of range", r.Name, r.Port)
			}
			if net.ParseIP(r.Target) == nil && !validHostname(r.Target) {
				return fmt.Errorf("route %q: invalid target %q", r.Name, r.Target)
			}
		default:
			return fmt.Errorf("route %q: needs a port or a cname", r.Name)
		}
	}
	return nil
}

// byName indexes routes by name for CNAME resolution.
func (c *Config) byName() map[string]Route {
	m := make(map[string]Route, len(c.Routes))
	for _, r := range c.Routes {
		m[r.Name] = r
	}
	return m
}

// resolve follows a route's CNAME chain to the terminal port-backed route,
// rejecting missing targets and cycles.
func (c *Config) resolve(start string) (Route, error) {
	byName := c.byName()
	seen := make(map[string]bool)
	name := start
	for {
		r, ok := byName[name]
		if !ok {
			return Route{}, fmt.Errorf("route %q: cname target %q not found", start, name)
		}
		if !r.isCname() {
			return r, nil
		}
		if seen[name] {
			return Route{}, fmt.Errorf("route %q: cname cycle through %q", start, name)
		}
		seen[name] = true
		name = r.Cname
	}
}

// Host returns the fully-qualified hostname for a route, e.g. "csl.this".
func (r Route) Host(suffix string) string { return r.Name + "." + suffix }

func (c *Config) hostFor(r Route) string { return r.Host(c.Suffix) }

// Hosts returns every resolved hostname in the config (for /etc/hosts sync).
func (c *Config) Hosts() []string {
	hosts := make([]string, 0, len(c.Routes))
	for _, r := range c.Routes {
		hosts = append(hosts, c.hostFor(r))
	}
	return hosts
}

// Backend returns the "host:port" dial target for a port-backed route.
func (r Route) Backend() string { return fmt.Sprintf("%s:%d", r.Target, r.Port) }

// BackendFor returns the dial target for a route, resolving a CNAME alias to
// its terminal port-backed route's backend.
func (c *Config) BackendFor(r Route) (string, error) {
	if !r.isCname() {
		return r.Backend(), nil
	}
	term, err := c.resolve(r.Name)
	if err != nil {
		return "", err
	}
	return term.Backend(), nil
}

// Site is one navigable ".this" site, served to the ⌘K picker as JSON. The
// JSON shape (name/host/url) is the picker's contract; Backend is internal and
// used only to probe the site's liveness before listing it.
type Site struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	URL     string `json:"url"`
	Backend string `json:"-"` // "host:port" of the backing service; never serialized
}

// Sites returns the port-backed routes as navigable sites, in config order.
// CNAME aliases are skipped — they point at a site already listed, so including
// them would just duplicate a destination in the picker.
func (c *Config) Sites() []Site {
	sites := make([]Site, 0, len(c.Routes))
	for _, r := range c.Routes {
		if r.isCname() {
			continue
		}
		host := c.hostFor(r)
		sites = append(sites, Site{
			Name:    r.Name,
			Host:    host,
			URL:     "http://" + host + "/",
			Backend: r.Backend(),
		})
	}
	return sites
}

// RouteMap returns a host -> backend map for the proxy (CNAMEs resolved).
func (c *Config) RouteMap() map[string]string {
	m := make(map[string]string, len(c.Routes))
	for _, r := range c.Routes {
		backend, err := c.BackendFor(r)
		if err != nil {
			continue // Load validated already; skip defensively
		}
		m[c.hostFor(r)] = backend
	}
	return m
}

// validHostname reports whether s is a valid dotted hostname (each label
// RFC 1123, total <= 253, no empty labels).
func validHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}
