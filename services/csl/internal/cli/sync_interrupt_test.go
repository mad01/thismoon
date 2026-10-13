package cli

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestInterruptContext_SignalCancels(t *testing.T) {
	ctx, stop := interruptContext(context.Background())
	defer stop()

	// The handler is registered before interruptContext returns, so this
	// signal reaches the context instead of killing the test binary.
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGINT did not cancel the sync context")
	}
}
