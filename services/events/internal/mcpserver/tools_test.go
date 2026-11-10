package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/events/internal/client"
	"github.com/mad01/thismoon/services/events/internal/event"
	"github.com/mad01/thismoon/services/events/internal/server"
	"github.com/mad01/thismoon/services/events/internal/store"
)

// newStubServer stands in for `events serve`: it filters the seeded events the
// way the real store does (exact source/level, substring q, after_id-exclusive)
// and reports the seeded source counts. It ignores since, which the tests
// against the real serve below cover, and records the last query it served.
func newStubServer(t *testing.T, events []client.Event, sources []client.SourceCount) *handlers {
	h, _ := newRecordingStub(t, events, sources)
	return h
}

// newRecordingStub is newStubServer plus the URL query of the last request.
func newRecordingStub(
	t *testing.T,
	events []client.Event,
	sources []client.SourceCount,
) (*handlers, *url.Values) {
	t.Helper()
	mux := http.NewServeMux()
	last := &url.Values{}

	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		*last = q
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
			if after := q.Get("after_id"); after != "" && e.ID <= after {
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
	return &handlers{client: client.New(srv.URL), webURL: srv.URL}, last
}

// newServeBacked runs the tools against the real serve handler over a temp
// store, whose clock stamps each emitted event with the next of stamps.
func newServeBacked(t *testing.T, stamps ...time.Time) *handlers {
	t.Helper()
	next := 0
	clock := func() time.Time {
		at := stamps[min(next, len(stamps)-1)]
		next++
		return at
	}
	st, err := store.New(t.TempDir(), 0, 0, clock)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	srv := httptest.NewServer(server.New(st, buildinfo.Info{}).Handler())
	t.Cleanup(srv.Close)
	return &handlers{client: client.New(srv.URL), webURL: srv.URL}
}

func mustEmit(t *testing.T, h *handlers, title string) string {
	t.Helper()
	_, out, err := h.handleEmit(context.Background(), nil, emitInput{Source: "deps", Title: title})
	if err != nil {
		t.Fatalf("emit %q: %v", title, err)
	}
	return out.ID
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

// TestQuerySinceSpanKeepsRecentEvents is the reported failure: since: "24h"
// came back as an empty success because since was read as an id cursor.
func TestQuerySinceSpanKeepsRecentEvents(t *testing.T) {
	now := time.Now()
	h := newServeBacked(t, now.Add(-48*time.Hour), now.Add(-time.Hour))
	mustEmit(t, h, "old")
	mustEmit(t, h, "recent")

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{Since: "24h"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Events) != 1 || res.Events[0].Title != "recent" {
		t.Errorf("since 24h = %+v, want only the recent event", res.Events)
	}

	_, res, err = h.handleQuery(context.Background(), nil, queryInput{Since: "7d"})
	if err != nil || len(res.Events) != 2 {
		t.Errorf("since 7d = %d events, %v; want both", len(res.Events), err)
	}
}

func TestQueryAfterIDPollsAgainstServe(t *testing.T) {
	now := time.Now()
	h := newServeBacked(t, now.Add(-2*time.Minute), now.Add(-time.Minute))
	first := mustEmit(t, h, "first")
	mustEmit(t, h, "second")

	for _, in := range []queryInput{{AfterID: first}, {Since: first}} {
		_, res, err := h.handleQuery(context.Background(), nil, in)
		if err != nil {
			t.Fatalf("query %+v: %v", in, err)
		}
		if len(res.Events) != 1 || res.Events[0].Title != "second" {
			t.Errorf("query %+v = %+v, want only the event after the cursor", in, res.Events)
		}
	}
}

// TestQueryWrongShapeNamesTheFix holds each bad value to an error that says
// which parameter would take it.
func TestQueryWrongShapeNamesTheFix(t *testing.T) {
	h := newServeBacked(t, time.Now())
	mustEmit(t, h, "x")

	tests := []struct {
		name string
		in   queryInput
		want []string
	}{
		{
			"since not a window",
			queryInput{Since: "yesterday"},
			[]string{"since", "24h", "after_id"},
		},
		{"after_id not an id", queryInput{AfterID: "24h"}, []string{"after_id", "since=24h"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := h.handleQuery(context.Background(), nil, tc.in)
			if err == nil {
				t.Fatal("want an error, got an empty success")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not mention %q", err, w)
				}
			}
		})
	}
}

func TestQueryLegacySinceIDMovesToAfterID(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	stamp := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	cursor := event.NewID(stamp, strings.NewReader("xx"))
	events := []client.Event{{ID: "0", Source: "deps", Level: "info", Title: "x"}}
	h, last := newRecordingStub(t, events, sources)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{Since: cursor})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got := last.Get("after_id"); got != cursor || last.Get("since") != "" {
		t.Errorf("sent since=%q after_id=%q, want the id as after_id", last.Get("since"), got)
	}
	if hint := res.ZeroHint; hint == nil || hint.AfterIDTime != stamp.Format(time.RFC3339) ||
		hint.SinceTime != "" {
		t.Errorf("hint = %+v, want after_id_time set and since_time empty", hint)
	}
}

func TestQuerySinceWindowResolvesInHint(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	events := []client.Event{{ID: "1", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(
		context.Background(), nil, queryInput{Since: "24h", Level: "error"},
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	at, err := time.Parse(time.RFC3339, hint.SinceTime)
	if err != nil || time.Since(at) < 23*time.Hour || time.Since(at) > 25*time.Hour {
		t.Errorf("since_time = %q, want about 24h ago", hint.SinceTime)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "since window") ||
		!strings.Contains(hint.Notes[0], "widen since") {
		t.Errorf("notes = %v, want the since-window note", hint.Notes)
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
		queryInput{Source: "deps", AfterID: cursor, Level: "error"},
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
	if !strings.Contains(note, "after_id cursor") || !strings.Contains(note, "level filter") {
		t.Errorf("note = %q, want both active filters named", note)
	}
}

func TestQueryAfterIDDecodesToTime(t *testing.T) {
	sources := []client.SourceCount{{Source: "deps", Count: 1}}
	stamp := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	cursor := event.NewID(stamp, strings.NewReader("xx"))
	events := []client.Event{{ID: "0", Source: "deps", Level: "info", Title: "x"}}
	h := newStubServer(t, events, sources)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{AfterID: cursor})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if hint.AfterIDTime != stamp.Format(time.RFC3339) {
		t.Errorf("after_id_time = %q, want %q", hint.AfterIDTime, stamp.Format(time.RFC3339))
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "drop after_id") {
		t.Errorf("notes = %v, want the after_id-cursor note", hint.Notes)
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
