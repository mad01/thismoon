// Package ref owns wire's connection string: the single token one session
// hands to another to bring it into a conversation. The server emits it and
// the client accepts it anywhere a channel is named, so the whole handoff is
// one copy and paste with nothing else to explain.
//
// The format is a URL with wire's own scheme:
//
//	wire://localhost:7432/refactor-auth
//
// The endpoint in the string is advisory. wire binds loopback only, so every
// channel today lives on the machine reading the string, and a client connects
// wherever it is configured (WIRE_PORT / --port) rather than where the string
// points. If channels ever become reachable across machines, the endpoint is
// already in the token and Parse is the one place that has to start honoring
// it.
package ref

import (
	"fmt"
	"net/url"
	"strings"
)

// Scheme marks a string as a wire connection string.
const Scheme = "wire"

// String builds the connection string for a channel served on port.
func String(port int, name string) string {
	return fmt.Sprintf("%s://localhost:%d/%s", Scheme, port, name)
}

// Parse reduces a channel reference to the id or name the API resolves. A
// plain id or name passes through untouched; a connection string is reduced to
// its channel component. Anything that looks like a URL but is not a wire one
// is an error rather than a silent fallthrough, because a mistyped scheme
// would otherwise become a nonsense channel name and fail much later.
func Parse(s string) (string, error) {
	ref := strings.TrimSpace(s)
	if ref == "" {
		return "", fmt.Errorf("channel is required")
	}
	if !strings.Contains(ref, "://") {
		return ref, nil
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid channel reference %q: %w", s, err)
	}
	if u.Scheme != Scheme {
		return "", fmt.Errorf(
			"invalid channel reference %q: expected a %s:// connection string, got %s://",
			s, Scheme, u.Scheme,
		)
	}
	name := strings.Trim(u.Path, "/")
	if name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf(
			"invalid connection string %q: expected %s://<host>:<port>/<channel>", s, Scheme,
		)
	}
	return name, nil
}
