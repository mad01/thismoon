// Package notify archives an event to the local events service. It is the
// shared form of the ~25-line internal/notify helper every component used
// to carry its own copy of, from back when each tool was a separate Go
// module and a shared package would have needed require+replace coupling.
//
// Every emit is best effort in both directions: a failed POST is dropped
// without a log line or an error return, because no caller here has a
// reason to fail on an unreachable archive. Nothing is emitted from tests,
// which would otherwise pollute the live timeline during `go test`.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// DefaultBaseURL is where the events service listens when EVENTS_BASE_URL
// is unset.
const DefaultBaseURL = "http://127.0.0.1:7430"

// Emit budgets. The synchronous path is tighter on purpose: its caller is
// blocked on it, and a hook that runs on every tool call cannot spend two
// seconds on an archive write.
const (
	asyncTimeout = 2 * time.Second
	syncTimeout  = time.Second
)

// Event is the archive payload the events service accepts on
// POST /api/events.
type Event struct {
	Source  string            `json:"source"`
	Level   string            `json:"level"`
	Title   string            `json:"title"`
	Message string            `json:"message"`
	Tags    map[string]string `json:"tags"`
}

// EmitEvent archives an event without blocking the caller: the POST runs
// in a goroutine and its outcome is discarded. Use it from servers and
// long-running commands. A process that exits right after emitting wants
// EmitEventSync instead, since the goroutine dies with the process.
func EmitEvent(source, level, title, message string, tags map[string]string) {
	if testing.Testing() {
		return
	}
	ev := Event{Source: source, Level: level, Title: title, Message: message, Tags: tags}
	go func() { _ = post(context.Background(), BaseURL(), ev, asyncTimeout) }()
}

// EmitEventSync archives an event and waits for the POST to land, up to a
// one-second budget. Use it from CLIs and hooks that exit immediately
// afterwards, where a goroutine would be killed before the request goes
// out.
func EmitEventSync(source, level, title, message string, tags map[string]string) {
	if testing.Testing() {
		return
	}
	ev := Event{Source: source, Level: level, Title: title, Message: message, Tags: tags}
	_ = post(context.Background(), BaseURL(), ev, syncTimeout)
}

// BaseURL returns the events service base URL: EVENTS_BASE_URL when set,
// otherwise DefaultBaseURL.
func BaseURL() string {
	if v := os.Getenv("EVENTS_BASE_URL"); v != "" {
		return v
	}
	return DefaultBaseURL
}

// post sends one event to base within timeout. Its error is for tests; the
// exported emitters discard it.
func post(ctx context.Context, base string, ev Event, timeout time.Duration) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("notify: encode event: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		base+"/api/events",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify: post event: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("notify: events service returned %d", resp.StatusCode)
	}
	return nil
}
