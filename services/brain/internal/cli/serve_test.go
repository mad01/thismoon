package cli

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

func TestServeRefusesABadPort(t *testing.T) {
	testhome.Isolate(t)
	if got := runBrain(t, "", "serve", "--port", "-1"); got.code != exitUsage ||
		!strings.Contains(got.stderr, "--port must be 0 to 65535") {
		t.Errorf("--port -1: exit %d, stderr %q", got.code, got.stderr)
	}
	// The env var sets the default, so a bad one fails the same way.
	t.Setenv(brain.PortEnv, "70000")
	if got := runBrain(t, "", "serve"); got.code != exitUsage ||
		!strings.Contains(got.stderr, "70000") {
		t.Errorf("%s=70000: exit %d, stderr %q", brain.PortEnv, got.code, got.stderr)
	}
}

func TestListenStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- listen(ctx, http.NotFoundHandler(), 0) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("listen = %v, want nil after shutdown", err)
		}
	case <-time.After(shutdownTimeout + time.Second):
		t.Fatal("listen did not return after its context ended")
	}
}

func TestListenAddrIsLoopback(t *testing.T) {
	if got := listenAddr(brain.DefaultPort); got != "127.0.0.1:7427" {
		t.Errorf("listenAddr = %q, want 127.0.0.1:7427", got)
	}
}
