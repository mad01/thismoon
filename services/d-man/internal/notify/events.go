package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

// EmitEvent best-effort archives a d-man event to the local events service
// (events.this). Fire-and-forget: it runs in a goroutine, never blocks the
// caller, and never reports an error — if `events serve` is down or absent the
// event is simply dropped.
//
// NOTE: this is a deliberate per-tool copy of the same ~25-line helper that
// reminder, deps, t-man, status, present, and speak each carry. The tools are
// separate Go modules (and t-man is a separate repo), so a shared package would
// need require+replace coupling across module boundaries; the copy is cheaper.
func EmitEvent(source, level, title, message string, tags map[string]string) {
	// Never emit from tests: this POSTs to the real events service on localhost,
	// which would pollute the live timeline during `go test`.
	if testing.Testing() {
		return
	}
	go func() {
		body, err := json.Marshal(map[string]any{
			"source":  source,
			"level":   level,
			"title":   title,
			"message": message,
			"tags":    tags,
		})
		if err != nil {
			return
		}
		base := os.Getenv("EVENTS_BASE_URL")
		if base == "" {
			base = "http://127.0.0.1:7430"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/events", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		_ = resp.Body.Close()
	}()
}
