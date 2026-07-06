// Package notify archives csl activity to the local events service
// (events.this) so sync runs and reindexes show up on the shared timeline.
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

// EmitEvent best-effort archives a csl event to the local events service
// (events.this). Unlike the long-running serve tools (present, reminder, deps,
// t-man, status, d-man, speak), csl commands exit right after finishing, so a
// fire-and-forget goroutine would be killed mid-POST — this copy is synchronous
// with a short timeout instead. It still never reports an error: if `events
// serve` is down or absent the event is simply dropped.
//
// NOTE: this is a deliberate per-tool copy of the same ~25-line helper the
// other tools carry. The tools are separate Go modules, so a shared package
// would need require+replace coupling across module boundaries; the copy is
// cheaper.
func EmitEvent(source, level, title, message string, tags map[string]string) {
	// Never emit from tests: this POSTs to the real events service on localhost,
	// which would pollute the live timeline during `go test`.
	if testing.Testing() {
		return
	}
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
}
