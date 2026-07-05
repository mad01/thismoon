package cli

import (
	"strings"
	"testing"
)

// TestListenAddrBindsLoopback guards against regressing to a bare ":<port>"
// bind, which would listen on all interfaces (0.0.0.0) and expose pages to the
// local network. The server must stay reachable only from localhost.
func TestListenAddrBindsLoopback(t *testing.T) {
	addr := listenAddr(7423)
	if addr != "127.0.0.1:7423" {
		t.Fatalf("listenAddr(7423) = %q, want 127.0.0.1:7423", addr)
	}
	if strings.HasPrefix(addr, ":") {
		t.Fatalf("listenAddr returned all-interfaces bind %q; must pin loopback", addr)
	}
}
