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

func TestCheckBind(t *testing.T) {
	cases := []struct {
		name     string
		shared   bool
		bind     string
		explicit bool
		wantErr  bool
	}{
		{"local default loopback", false, "127.0.0.1", false, false},
		{"local localhost", false, "localhost", true, false},
		{"local ipv6 loopback", false, "::1", true, false},
		{"local all interfaces", false, "0.0.0.0", true, true},
		{"local lan address", false, "192.168.1.10", true, true},
		{"shared without explicit bind", true, "127.0.0.1", false, true},
		{"shared explicit loopback", true, "127.0.0.1", true, false},
		{"shared explicit all interfaces", true, "0.0.0.0", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkBind(tc.shared, tc.bind, tc.explicit)
			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"checkBind(%v, %q, %v) = %v, wantErr %v",
					tc.shared,
					tc.bind,
					tc.explicit,
					err,
					tc.wantErr,
				)
			}
		})
	}
}

func TestBindAddr(t *testing.T) {
	if got := bindAddr("0.0.0.0", 7423); got != "0.0.0.0:7423" {
		t.Errorf("bindAddr = %q", got)
	}
	if got := bindAddr("::1", 7423); got != "[::1]:7423" {
		t.Errorf("bindAddr ipv6 = %q", got)
	}
}
