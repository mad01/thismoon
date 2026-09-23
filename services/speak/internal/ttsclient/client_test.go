package ttsclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/speak/internal/tts"
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

	data, err := New(srv.URL).Synthesize(context.Background(), "hello world", "af_heart")
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

	if _, err := New(srv.URL).Synthesize(context.Background(), "x", "af_heart"); err == nil {
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

// TestSynthesizeClassifiesFailures pins the contract every surface builds
// its "why" message on: each failure is a *tts.Error whose kind says what to
// fix and whose message carries the engine's own reason.
func TestSynthesizeClassifiesFailures(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantKind tts.Kind
		wantMsg  string
	}{
		{
			"auth",
			http.StatusUnauthorized,
			`{"error":{"message":"bad key"}}`,
			tts.KindAuth,
			"bad key",
		},
		{
			"missing model",
			http.StatusNotFound,
			`{"detail":"model not found"}`,
			tts.KindModel,
			"model not found",
		},
		{"rate limited", http.StatusTooManyRequests, "slow down", tts.KindQuota, "slow down"},
		{
			"engine crash",
			http.StatusInternalServerError,
			`{"detail":"LocalEntryNotFoundError: Kokoro-82M"}`,
			tts.KindUpstream,
			"returned 500: LocalEntryNotFoundError: Kokoro-82M",
		},
		{"empty audio", http.StatusOK, "", tts.KindUpstream, "empty audio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}),
			)
			defer srv.Close()

			_, err := New(srv.URL).Synthesize(context.Background(), "x", "af_heart")
			te, ok := errors.AsType[*tts.Error](err)
			if !ok {
				t.Fatalf("error = %v, want a *tts.Error", err)
			}
			if te.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", te.Kind, tc.wantKind)
			}
			if !strings.Contains(te.Message, tc.wantMsg) {
				t.Errorf("message = %q, want it to contain %q", te.Message, tc.wantMsg)
			}
		})
	}
}

func TestSynthesizeUnreachableIsNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := New(url).Synthesize(context.Background(), "x", "af_heart")
	te, ok := errors.AsType[*tts.Error](err)
	if !ok || te.Kind != tts.KindNetwork {
		t.Fatalf("error = %v, want a network *tts.Error", err)
	}
	if !strings.Contains(err.Error(), "speak doctor") {
		t.Errorf("error = %q, want the doctor hint", err)
	}
}

// TestSynthesizeBrokenStreamIsUpstream pins that an engine which answers and
// then breaks off counts as reachable-but-failing: doctor skips the
// synthesis check for network errors, so a network kind here would hide the
// failure behind "engine not reachable".
func TestSynthesizeBrokenStreamIsUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RIFF"))
	}))
	defer srv.Close()

	_, err := New(srv.URL).Synthesize(context.Background(), "x", "af_heart")
	te, ok := errors.AsType[*tts.Error](err)
	if !ok || te.Kind != tts.KindUpstream ||
		!strings.Contains(te.Message, "broke off the audio stream") {
		t.Fatalf("error = %v, want an upstream broken-stream *tts.Error", err)
	}
}
