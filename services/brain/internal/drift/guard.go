package drift

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
)

// maxRedirects is how many redirects one fetch follows, as net/http does
// by default.
const maxRedirects = 10

// sharedAddressSpace is 100.64.0.0/10 (RFC 6598), the carrier-grade NAT
// range that overlay networks also hand out to private hosts. netip has no
// predicate for it.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// errLocalAddress marks a connection refused because it would reach a
// loopback, link-local or private address.
var errLocalAddress = errors.New("refused a loopback, link-local or private address")

// newClient returns the client drift fetches external sources with. Unless
// allowLocal is set, every connection it dials is checked by refuseLocal,
// and it ignores proxy settings so that the check sees the source's own
// address rather than a proxy's.
func newClient(allowLocal bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if !allowLocal {
		dialer := &net.Dialer{Timeout: FetchTimeout, Control: refuseLocal}
		transport.DialContext = dialer.DialContext
		transport.Proxy = nil
	}
	return &http.Client{
		Timeout:       FetchTimeout,
		Transport:     transport,
		CheckRedirect: webRedirectsOnly,
	}
}

// refuseLocal is a net.Dialer Control hook. It runs on the resolved address
// of each connection, so a host name that resolves, or rebinds, to a local
// address is refused as well as a literal one.
func refuseLocal(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("dial %s: %w", address, err)
	}
	if ip := ap.Addr().Unmap(); isLocal(ip) {
		return fmt.Errorf("dial %s: %w (allow it with --allow-local)", ip, errLocalAddress)
	}
	return nil
}

// isLocal reports whether ip is one a shared vault's sources must not make
// drift reach: loopback, unspecified, private, link-local, multicast or the
// shared address space.
func isLocal(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsMulticast() || sharedAddressSpace.Contains(ip)
}

// webRedirectsOnly follows a redirect only to another http or https URL.
func webRedirectsOnly(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("redirect to %s: drift fetches only http and https", req.URL.Redacted())
	}
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	return nil
}
