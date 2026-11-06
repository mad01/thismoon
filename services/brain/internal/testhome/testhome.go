// Package testhome isolates a test from the machine's brain config and
// state. Tests import it; the binary never does.
package testhome

import (
	"net"
	"strconv"
	"testing"

	"github.com/mad01/thismoon/services/brain"
)

// Isolate points HOME at a fresh temp dir and clears every variable that
// could route brain to a real config or state directory, then returns the
// new home. It also points brain.PortEnv at a loopback port nothing listens
// on, so brain doctor never probes a brain serve running on the machine. It
// uses t.Setenv, so the caller cannot run in parallel.
func Isolate(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv(brain.ConfigEnv, "")
	t.Setenv(brain.PortEnv, strconv.Itoa(closedPort(t)))
	return home
}

// closedPort returns a loopback port that was free a moment ago: the port
// of a listener it opened and closed.
func closedPort(t testing.TB) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("testhome: find a free port: %v", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if err := ln.Close(); err != nil {
		t.Fatalf("testhome: close the probe listener: %v", err)
	}
	if !ok {
		t.Fatalf("testhome: listener address %v is not TCP", ln.Addr())
	}
	return addr.Port
}
