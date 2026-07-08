package ttsclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSynthesizeRequestsWAV(t *testing.T) {
	var got speechRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/audio/speech" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte("RIFFwavdata"))
	}))
	defer srv.Close()

	data, err := New(srv.URL).Synthesize("hello world", "af_heart")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got.ResponseFormat != "wav" {
		t.Errorf("response_format = %q, want wav", got.ResponseFormat)
	}
	if got.Input != "hello world" || got.Voice != "af_heart" {
		t.Errorf("payload = %+v", got)
	}
	if string(data) != "RIFFwavdata" {
		t.Errorf("audio bytes = %q", data)
	}
}

func TestSynthesizeEmptyBodyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 200 with empty body — the ffmpeg-missing failure mode
	}))
	defer srv.Close()

	if _, err := New(srv.URL).Synthesize("x", "af_heart"); err == nil {
		t.Fatal("expected error on empty audio, got nil")
	}
}

func TestReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !New(srv.URL).Reachable() {
		t.Error("Reachable() = false for a live server")
	}
	srv.Close()
	if New(srv.URL).Reachable() {
		t.Error("Reachable() = true for a dead server")
	}
}
