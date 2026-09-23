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
	go func() { served <- serveUntilDone(ctx, ln, handler, nil) }()

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

// TestServeUntilDoneEndsStreamsOnShutdown pins the hook event streams rely
// on: a response that never finishes by itself would hold the drain for its
// whole timeout, so onShutdown runs as shutdown starts and the stream ends
// well inside the drain.
func TestServeUntilDoneEndsStreamsOnShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	streaming, stopStreams := make(chan struct{}), make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "event: version\ndata: 1\n\n")
		_ = http.NewResponseController(w).Flush()
		close(streaming)
		<-stopStreams
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- serveUntilDone(ctx, ln, handler, func() { close(stopStreams) })
	}()

	resp, err := http.Get("http://" + addr + "/events")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	<-streaming

	start := time.Now()
	cancel()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("serveUntilDone = %v, want nil", err)
		}
	case <-time.After(drainTimeout):
		t.Fatalf("shutdown waited out the %s drain on an open stream", drainTimeout)
	}
	if took := time.Since(start); took > drainTimeout/2 {
		t.Errorf("shutdown took %s with an open stream, want well under %s", took, drainTimeout)
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
