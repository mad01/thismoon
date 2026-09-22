package cli

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestServeUntilDoneDrainsInFlightRequests pins the shutdown contract a
// rolling update depends on: once the context ends the listener stops
// accepting, but a request already being served runs to completion instead
// of having its connection reset.
func TestServeUntilDoneDrainsInFlightRequests(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	entered, release := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "drained")
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serveUntilDone(ctx, ln, handler) }()

	type reply struct {
		code int
		body string
		err  error
	}
	replies := make(chan reply, 1)
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	go func() {
		resp, err := client.Get("http://" + addr + "/slow")
		if err != nil {
			replies <- reply{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		replies <- reply{code: resp.StatusCode, body: string(body), err: err}
	}()

	<-entered
	cancel()
	waitUntilRefused(t, addr)
	close(release)

	got := <-replies
	if got.err != nil {
		t.Fatalf("in-flight request: %v", got.err)
	}
	if got.code != http.StatusOK || got.body != "drained" {
		t.Errorf("in-flight request = %d %q, want 200 \"drained\"", got.code, got.body)
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serveUntilDone = %v, want nil after a clean shutdown", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serveUntilDone did not return after its context was cancelled")
	}
}

// waitUntilRefused blocks until the address stops accepting connections,
// which is what shutdown does first.
func waitUntilRefused(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s still accepts connections after shutdown started", addr)
}
