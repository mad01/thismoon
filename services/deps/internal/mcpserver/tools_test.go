package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/deps/internal/client"
)

// newStubHandlers stands in for `deps serve`, answering GET /api/flagged with
// the given result.
func newStubHandlers(t *testing.T, res client.CheckResult) *handlers {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/flagged", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &handlers{client: client.New(srv.URL), webURL: srv.URL}
}

func TestListFlaggedEmptyStoreCarriesZeroHint(t *testing.T) {
	h := newStubHandlers(t, client.CheckResult{})

	_, out, err := h.handleListFlagged(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("list flagged: %v", err)
	}
	hint := out.ZeroHint
	if hint == nil || len(hint.Notes) != 1 {
		t.Fatalf("zero_result_hint = %+v, want the never-ran note", hint)
	}
	if !strings.Contains(hint.Notes[0], "no scan or check has completed") {
		t.Errorf("note = %q, want the never-ran wording", hint.Notes[0])
	}
}

func TestListFlaggedScannedButEmptyCatalogCarriesDiscoveredZeroNote(t *testing.T) {
	scanned := time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
	h := newStubHandlers(t, client.CheckResult{ScannedAt: scanned})

	_, out, err := h.handleListFlagged(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("list flagged: %v", err)
	}
	hint := out.ZeroHint
	if hint == nil || len(hint.Notes) != 1 {
		t.Fatalf("zero_result_hint = %+v, want the discovered-zero note", hint)
	}
	if !strings.Contains(hint.Notes[0], "discovered zero dependencies") {
		t.Errorf("note = %q, want the discovered-zero wording", hint.Notes[0])
	}
}

func TestListFlaggedCleanStoreHasNoZeroHint(t *testing.T) {
	h := newStubHandlers(t, client.CheckResult{Total: 500})

	_, out, err := h.handleListFlagged(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("list flagged: %v", err)
	}
	if out.ZeroHint != nil {
		t.Errorf(
			"zero_result_hint must be absent when total > 0 (genuinely clean), got %v",
			out.ZeroHint,
		)
	}
}

// TestToolAnnotationContract holds every registered tool to the repo-wide
// annotation rules: a spec-legal name, a description, an explicit open-world
// hint, and a destructive hint on anything that writes.
func TestToolAnnotationContract(t *testing.T) {
	s, err := New("test", Config{
		Port:    7429,
		BaseURL: "http://deps.this",
		Checks:  func(context.Context) []doctor.Check { return nil },
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	mcptest.VerifyToolAnnotations(t, s)
}
