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
	"github.com/mad01/thismoon/services/events/internal/client"
	"github.com/mad01/thismoon/services/events/internal/event"
)

// newStubServer stands in for `events serve`: it filters the seeded events the
// way the real store does (exact source/level, substring q, since-exclusive)
// and reports the seeded source counts.
func newStubServer(t *testing.T, events []client.Event, sources []client.SourceCount) *handlers {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		list := make([]client.Event, 0, len(events))
		for _, e := range events {
			if s := q.Get("source"); s != "" && e.Source != s {
				continue
			}
			if l := q.Get("level"); l != "" && e.Level != l {
				continue
			}
			if sub := q.Get("q"); sub != "" && !strings.Contains(e.Title, sub) {
				continue
			}
			if since := q.Get("since"); since != "" && e.ID <= since {
				continue
			}
			list = append(list, e)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(list)
	})

	mux.HandleFunc("GET /api/sources", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sources)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &handlers{client: client.New(srv.URL), webURL: srv.URL}
}

func TestQueryEmptyLogCarriesZeroHint(t *testing.T) {
	h := newStubServer(t, nil, nil)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint on empty log")
	}
	if hint.EventsStored != 0 {
		t.Errorf("events_stored = %d, want 0", hint.EventsStored)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "nothing has been emitted") {
		t.Errorf("notes = %v, want the empty-log note", hint.Notes)
	}
}

func TestQueryUnknownSourceCarriesKnownSources(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 3}, {Source: "reminder", Count: 2}}
	events := []client.Event{{ID: "1", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{Source: "nosuch"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if hint.SourceExists {
		t.Error("source_exists = true for unknown source, want false")
	}
	if len(hint.KnownSources) != 2 || hint.EventsStored != 5 {
		t.Errorf("hint = %+v, want 2 known sources and 5 events stored", hint)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "deps, reminder") {
		t.Errorf("notes = %v, want the known-sources note", hint.Notes)
	}
}

func TestQueryFiltersExcludedEventsCarriesFilterNote(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	events := []client.Event{{ID: "1", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(
		context.Background(),
		nil,
		queryInput{Source: "deps", Level: "error"},
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if !hint.SourceExists {
		t.Error("source_exists = false for existing source, want true")
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "level filter excluded") {
		t.Errorf("notes = %v, want the filter-excluded note", hint.Notes)
	}
}

func TestQueryCapitalizedSourceMatchesSanitizedName(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	events := []client.Event{{ID: "1", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(
		context.Background(),
		nil,
		queryInput{Source: "Deps", Level: "error"},
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if !hint.SourceExists {
		t.Error("source_exists = false for a source that matches after sanitizing, want true")
	}
	if len(hint.Notes) != 1 || strings.Contains(hint.Notes[0], "does not exist") {
		t.Errorf("notes = %v, must not claim the source does not exist", hint.Notes)
	}
}

func TestQueryMalformedSinceCursorCarriesCursorNote(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	events := []client.Event{{ID: "1", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{Since: "zzz-garbage"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if hint.SinceTime != "" {
		t.Errorf("since_time = %q for a malformed cursor, want empty", hint.SinceTime)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "does not decode") {
		t.Errorf("notes = %v, want the malformed-cursor note", hint.Notes)
	}
}

func TestQueryCombinedFiltersNamedTogether(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	stamp := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	cursor := event.NewID(stamp, strings.NewReader("xx"))
	events := []client.Event{{ID: "0", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(
		context.Background(),
		nil,
		queryInput{Source: "deps", Since: cursor, Level: "error"},
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if len(hint.Notes) != 1 {
		t.Fatalf("notes = %v, want one combined note", hint.Notes)
	}
	note := hint.Notes[0]
	if !strings.Contains(note, "since cursor") || !strings.Contains(note, "level filter") {
		t.Errorf("note = %q, want both active filters named", note)
	}
}

func TestQuerySinceCursorDecodesToTime(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	stamp := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	cursor := event.NewID(stamp, strings.NewReader("xx"))
	events := []client.Event{{ID: "0", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{Since: cursor})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if hint.SinceTime != stamp.Format(time.RFC3339) {
		t.Errorf("since_time = %q, want %q", hint.SinceTime, stamp.Format(time.RFC3339))
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "since cursor") {
		t.Errorf("notes = %v, want the since-cursor note", hint.Notes)
	}
}

func TestQueryNonEmptyResultHasNoZeroHint(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	events := []client.Event{{ID: "1", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("want 1 event, got %d", len(res.Events))
	}
	if res.ZeroHint != nil {
		t.Errorf("zero_result_hint must be absent on non-empty results, got %+v", res.ZeroHint)
	}
}

// TestToolAnnotationContract holds every registered tool to the repo-wide
// annotation rules: a spec-legal name, a description, an explicit open-world
// hint, and a destructive hint on anything that writes.
func TestToolAnnotationContract(t *testing.T) {
	s, err := New("test", Config{
		Port:    7430,
		BaseURL: "http://events.this",
		Checks:  func(context.Context) []doctor.Check { return nil },
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	mcptest.VerifyToolAnnotations(t, s)
}
