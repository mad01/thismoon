package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/mcptest"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
)

// stubStore is an in-memory hand-rolled stand-in for `kof serve`. It implements
// just enough of the HTTP contract to exercise the tool handlers end to end,
// without importing the real server package (built in parallel).
type stubStore struct {
	mu    sync.Mutex
	byID  map[string]client.Assertion
	order []string
	seq   int
}

func newStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := &stubStore{byID: map[string]client.Assertion{}}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/assertions", func(w http.ResponseWriter, r *http.Request) {
		var body client.AssertBody
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Pins) == 0 {
			writeErr(w, http.StatusBadRequest, "kof: at least one pin is required")
			return
		}
		s.mu.Lock()
		s.seq++
		id := fmt.Sprintf("a%d", s.seq)
		pins := make([]client.Pin, len(body.Pins))
		for i, p := range body.Pins {
			pins[i] = client.Pin{
				RepoPath:      p.RepoPath,
				File:          p.File,
				StartLine:     p.StartLine,
				EndLine:       p.EndLine,
				ContentSHA256: "deadbeef",
			}
		}
		a := client.Assertion{
			ID:         id,
			Kind:       body.Kind,
			Subject:    body.Subject,
			Statement:  body.Statement,
			Confidence: body.Confidence,
			Pins:       pins,
			Status:     "fresh",
		}
		s.byID[id] = a
		s.order = append(s.order, id)
		s.mu.Unlock()
		writeJSON(w, http.StatusCreated, a)
	})

	mux.HandleFunc("GET /api/assertions", func(w http.ResponseWriter, r *http.Request) {
		subject := r.URL.Query().Get("subject")
		kind := r.URL.Query().Get("kind")
		status := r.URL.Query().Get("status")
		s.mu.Lock()
		list := make([]client.Assertion, 0, len(s.order))
		for _, id := range s.order {
			a := s.byID[id]
			if subject != "" && !strings.HasPrefix(a.Subject, subject) {
				continue
			}
			if kind != "" && a.Kind != kind {
				continue
			}
			if status != "" && a.Status != status {
				continue
			}
			list = append(list, a)
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, struct {
			Assertions []client.Assertion `json:"assertions"`
		}{Assertions: list})
	})

	mux.HandleFunc("GET /api/assertions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		a, ok := s.byID[r.PathValue("id")]
		s.mu.Unlock()
		if !ok {
			writeErr(w, http.StatusNotFound, "kof: not found")
			return
		}
		writeJSON(w, http.StatusOK, a)
	})

	mux.HandleFunc(
		"POST /api/assertions/{id}/retract",
		func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Note string `json:"note"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.mu.Lock()
			a, ok := s.byID[r.PathValue("id")]
			if ok {
				a.Status = "retracted"
				a.RetractNote = body.Note
				s.byID[a.ID] = a
			}
			s.mu.Unlock()
			if !ok {
				writeErr(w, http.StatusNotFound, "kof: not found")
				return
			}
			writeJSON(w, http.StatusOK, a)
		},
	)

	mux.HandleFunc("POST /api/check", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		n := len(s.order)
		fresh := make([]client.Assertion, 0, n)
		for _, id := range s.order {
			fresh = append(fresh, s.byID[id])
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, client.CheckReport{
			Checked:    n,
			Fresh:      n,
			Assertions: fresh,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func testHandlers(t *testing.T) *handlers {
	t.Helper()
	srv := newStubServer(t)
	return &handlers{client: client.New(srv.URL), webURL: srv.URL}
}

func validPins() []pinInput {
	return []pinInput{{RepoPath: "mad01/thismoon", File: "kof.go", StartLine: 1, EndLine: 3}}
}

func TestAssertThenGetAndQuery(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()

	_, created, err := h.handleAssert(ctx, nil, assertInput{
		Kind:       "code-behavior",
		Subject:    "repo:mad01/thismoon/services/keeper-of-facts",
		Statement:  "serve is the single writer",
		Confidence: "verified",
		SessionID:  "sess-1",
		Pins:       validPins(),
	})
	if err != nil {
		t.Fatalf("assert: %v", err)
	}
	if created.Assertion.ID == "" || created.Assertion.Status != "fresh" {
		t.Fatalf("unexpected created: %+v", created.Assertion)
	}
	if created.URL == "" {
		t.Error("assert output missing url")
	}

	_, got, err := h.handleGet(ctx, nil, idInput{ID: created.Assertion.ID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Assertion.Statement != "serve is the single writer" || len(got.Assertion.Pins) != 1 {
		t.Errorf("get returned %+v", got.Assertion)
	}
	if got.URL == "" {
		t.Error("get output missing url")
	}

	_, list, err := h.handleQuery(ctx, nil, queryInput{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(list.Assertions) != 1 {
		t.Errorf("query len = %d, want 1", len(list.Assertions))
	}
	if list.URL == "" {
		t.Error("query output missing url")
	}
}

func TestAssertWithoutPinsSurfacesError(t *testing.T) {
	h := testHandlers(t)

	_, _, err := h.handleAssert(context.Background(), nil, assertInput{
		Kind:       "decision",
		Subject:    "repo:mad01/thismoon",
		Statement:  "no evidence",
		Confidence: "hint",
		SessionID:  "sess-1",
		Pins:       nil,
	})
	if err == nil {
		t.Fatal("want error when asserting with no pins")
	}
}

func TestRetractRecordsNote(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()

	_, created, _ := h.handleAssert(ctx, nil, assertInput{
		Kind: "code-behavior", Subject: "s", Statement: "x",
		Confidence: "derived", SessionID: "sess-1", Pins: validPins(),
	})

	_, res, err := h.handleRetract(ctx, nil, retractInput{
		ID:   created.Assertion.ID,
		Note: "superseded",
	})
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	if res.Assertion.Status != "retracted" || res.Assertion.RetractNote != "superseded" {
		t.Errorf("retract result = %+v", res.Assertion)
	}
	if res.URL == "" {
		t.Error("retract output missing url")
	}
}

func TestCheckReturnsCounts(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()

	_, _, _ = h.handleAssert(ctx, nil, assertInput{
		Kind: "code-behavior", Subject: "s", Statement: "x",
		Confidence: "derived", SessionID: "sess-1", Pins: validPins(),
	})

	_, res, err := h.handleCheck(ctx, nil, checkInput{})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if res.Checked != 1 || res.Fresh != 1 {
		t.Errorf("check result = %+v", res)
	}
	if res.URL == "" {
		t.Error("check output missing url")
	}
}

func TestServeUnreachableError(t *testing.T) {
	h := &handlers{client: client.New("http://127.0.0.1:0"), webURL: ""}
	_, _, err := h.handleQuery(context.Background(), nil, queryInput{})
	if err == nil {
		t.Fatal("want error when serve is unreachable")
	}
}

func TestQueryEmptyStoreCarriesZeroHint(t *testing.T) {
	h := testHandlers(t)

	_, res, err := h.handleQuery(context.Background(), nil, queryInput{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.ZeroHint == nil {
		t.Fatal("want zero_result_hint on empty store")
	}
	if res.ZeroHint.StoreAssertions != 0 {
		t.Errorf("store_assertions = %d, want 0", res.ZeroHint.StoreAssertions)
	}
	if len(res.ZeroHint.Notes) != 1 {
		t.Errorf("notes = %v, want the empty-store note", res.ZeroHint.Notes)
	}
}

func TestQuerySubjectPrefixMissCarriesShorterPrefixNote(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()

	_, _, _ = h.handleAssert(ctx, nil, assertInput{
		Kind: "code-behavior", Subject: "repo:mad01/thismoon/services/events",
		Statement: "x", Confidence: "derived", SessionID: "s", Pins: validPins(),
	})

	_, res, err := h.handleQuery(ctx, nil, queryInput{
		Subject: "repo:mad01/thismoon/services/nosuch",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Assertions) != 0 {
		t.Fatalf("want empty result, got %d", len(res.Assertions))
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if hint.SubjectsMatched != 0 || hint.SubjectsStored != 1 || hint.StoreAssertions != 1 {
		t.Errorf("hint = %+v, want matched=0 stored=1 assertions=1", hint)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], `"repo:mad01/thismoon/services"`) {
		t.Errorf("notes = %v, want a shorter-prefix suggestion", hint.Notes)
	}
}

func TestQueryFiltersExcludedMatchesCarriesFilterNote(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()

	_, _, _ = h.handleAssert(ctx, nil, assertInput{
		Kind: "code-behavior", Subject: "repo:mad01/thismoon",
		Statement: "x", Confidence: "derived", SessionID: "s", Pins: validPins(),
	})

	_, res, err := h.handleQuery(ctx, nil, queryInput{
		Subject: "repo:mad01/thismoon",
		Status:  "retracted",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	hint := res.ZeroHint
	if hint == nil {
		t.Fatal("want zero_result_hint")
	}
	if hint.SubjectsMatched != 1 {
		t.Errorf("subjects_matched = %d, want 1", hint.SubjectsMatched)
	}
	if len(hint.Notes) != 1 || !strings.Contains(hint.Notes[0], "kind/status") {
		t.Errorf("notes = %v, want the filter-excluded note", hint.Notes)
	}
}

func TestQueryNonEmptyResultHasNoZeroHint(t *testing.T) {
	h := testHandlers(t)
	ctx := context.Background()

	_, _, _ = h.handleAssert(ctx, nil, assertInput{
		Kind: "code-behavior", Subject: "repo:mad01/thismoon",
		Statement: "x", Confidence: "derived", SessionID: "s", Pins: validPins(),
	})

	_, res, err := h.handleQuery(ctx, nil, queryInput{Subject: "repo:mad01"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(res.Assertions) != 1 {
		t.Fatalf("want 1 assertion, got %d", len(res.Assertions))
	}
	if res.ZeroHint != nil {
		t.Errorf("zero_result_hint must be absent on non-empty results, got %+v", res.ZeroHint)
	}
}

// TestToolAnnotationsFollowTheContract builds the real MCP server and checks
// the tool list against the shared annotation gate, so a tool added without
// its read-only/destructive/open-world hints fails here rather than shipping
// with the SDK defaults.
func TestToolAnnotationsFollowTheContract(t *testing.T) {
	s, err := New("test", Config{
		Port:   1,
		Checks: func(context.Context) []doctor.Check { return nil },
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	mcptest.VerifyToolAnnotations(t, s)
}
