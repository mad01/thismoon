package jsdelivr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// cytoscapeSum is the sha256 of cytoscape 3.31.0's dist/cytoscape.min.js, as
// a component vendors it under static/; testdata/lookup-cytoscape-3.31.0.json
// is the API's recorded answer for it.
const cytoscapeSum = "e7d4c6c286bee0ba346c68ed1c8b8b82b4740b75be5d0c4d6602ee38df55d9c1"

// stub serves the recorded answers: the cytoscape hit for cytoscapeSum, the
// recorded 404 for any other hash, and status for the hash "fail". It counts
// the requests it receives.
func stub(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	hit := readFile(t, "testdata/lookup-cytoscape-3.31.0.json")
	miss := readFile(t, "testdata/lookup-miss.json")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/v1/lookup/hash/" + cytoscapeSum:
			_, _ = w.Write(hit)
		case "/v1/lookup/hash/fail":
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"status":503,"message":"try later"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(miss)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestLookupMapsTheRecordedHit(t *testing.T) {
	srv, _ := stub(t, http.StatusServiceUnavailable)
	file, found, err := NewWithURL(srv.URL).Lookup(context.Background(), cytoscapeSum)
	if err != nil || !found {
		t.Fatalf("Lookup = %+v, %v, %v; want a match", file, found, err)
	}
	want := File{Type: "npm", Name: "cytoscape", Version: "3.31.0", Path: "/dist/cytoscape.min.js"}
	if file != want {
		t.Errorf("file = %+v, want %+v", file, want)
	}
}

func TestLookupReadsA404AsAMiss(t *testing.T) {
	srv, _ := stub(t, http.StatusServiceUnavailable)
	file, found, err := NewWithURL(srv.URL).Lookup(context.Background(), "abc123")
	if err != nil || found || file != (File{}) {
		t.Errorf("Lookup = %+v, %v, %v; want a miss with no error", file, found, err)
	}
}

func TestLookupFailsOnAnyOtherStatus(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		srv, _ := stub(t, status)
		_, found, err := NewWithURL(srv.URL).Lookup(context.Background(), "fail")
		if err == nil || found {
			t.Errorf("status %d: found = %v, err = %v; want an error", status, found, err)
		}
	}
}

func TestLookupCachesHitsForGoodAndMissesForADay(t *testing.T) {
	srv, calls := stub(t, http.StatusServiceUnavailable)
	c := NewWithURL(srv.URL)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	lookup := func(sum string) {
		t.Helper()
		if _, _, err := c.Lookup(context.Background(), sum); err != nil {
			t.Fatalf("Lookup(%s): %v", sum, err)
		}
	}

	lookup(cytoscapeSum)
	lookup("abc123")
	lookup(cytoscapeSum)
	lookup("abc123")
	if got := calls.Load(); got != 2 {
		t.Fatalf("requests = %d after repeats, want 2 (one per hash)", got)
	}

	now = now.Add(missTTL)
	lookup(cytoscapeSum)
	lookup("abc123")
	if got := calls.Load(); got != 3 {
		t.Errorf("requests = %d a day later, want 3 (the miss asked again, the hit not)", got)
	}
}

func TestLookupNeverCachesAnError(t *testing.T) {
	srv, calls := stub(t, http.StatusServiceUnavailable)
	c := NewWithURL(srv.URL)
	for range 2 {
		if _, _, err := c.Lookup(context.Background(), "fail"); err == nil {
			t.Fatal("Lookup succeeded, want the 503 as an error")
		}
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("requests = %d, want 2 (a failed lookup is retried)", got)
	}
}
