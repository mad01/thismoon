package hint

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKofProbeCountsAssertions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/assertions" {
			t.Errorf("path = %q, want /api/assertions", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"assertions":[{"id":"a"},{"id":"b"}]}`))
	}))
	defer srv.Close()

	got, err := KofProbe(srv.URL)
	if err != nil {
		t.Fatalf("KofProbe: %v", err)
	}
	if got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
}

func TestKofProbeEmptyStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"assertions":[]}`))
	}))
	defer srv.Close()

	got, err := KofProbe(srv.URL)
	if err != nil {
		t.Fatalf("KofProbe: %v", err)
	}
	if got != 0 {
		t.Errorf("count = %d, want 0", got)
	}
}

func TestKofProbeErrors(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		if _, err := KofProbe(srv.URL); err == nil {
			t.Error("want error on 500 response, got nil")
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		srv.Close()

		if _, err := KofProbe(srv.URL); err == nil {
			t.Error("want error on closed server, got nil")
		}
	})
}
