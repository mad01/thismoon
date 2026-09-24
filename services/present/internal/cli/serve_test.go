package cli

import (
	"os"
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

// PRESENT_SPEAK_URL is the one PRESENT_* variable where an empty value is a
// setting: the shared instance's manifest sets it empty to keep pages there
// from probing speak.this, so it must not fall back to the default the way
// envdefault.String would.
func TestDefaultSpeakURL(t *testing.T) {
	cases := []struct {
		name  string
		set   bool
		value string
		want  string
	}{
		{"unset is the fleet speak", false, "", "http://speak.this"},
		{"set wins", true, "http://127.0.0.1:7426", "http://127.0.0.1:7426"},
		{"empty turns read-aloud off", true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("PRESENT_SPEAK_URL", tc.value)
			} else {
				t.Setenv("PRESENT_SPEAK_URL", "")
				_ = os.Unsetenv("PRESENT_SPEAK_URL")
			}
			if got := defaultSpeakURL(); got != tc.want {
				t.Errorf("defaultSpeakURL() = %q, want %q", got, tc.want)
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
