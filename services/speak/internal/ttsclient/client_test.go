package ttsclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// shortTimeout stands in for the real timeouts, so a slow endpoint times out
// fast.
const shortTimeout = 50 * time.Millisecond

// localClient is a client for a stand-in local engine at url.
func localClient(url string) *Client {
	return New(Config{
		Provider: "local",
		Local:    true,
		BaseURL:  url,
		Model:    "mlx-community/Kokoro-82M-bf16",
		Format:   "wav",
	})
}

func synthesize(t *testing.T, c *Client) (tts.Audio, error) {
	t.Helper()
	return c.Synthesize(context.Background(), tts.Request{Text: "hello world", Voice: "af_heart"})
}

// TestSynthesizeSendsTheOpenAIRequest pins the request every
// OpenAI-compatible endpoint gets: the configured model (the local engine
// answers 422 without one), the format, the bearer key, and the speed.
func TestSynthesizeSendsTheOpenAIRequest(t *testing.T) {
	var got speechRequest
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/audio/speech" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFwavdata"))
	}))
	defer srv.Close()

	c := New(Config{
		Provider: "openrouter",
		BaseURL:  srv.URL + "/api/",
		APIKey:   "sk-test",
		Model:    "hexgrad/kokoro-82m",
		Format:   "pcm",
	})
	audio, err := c.Synthesize(context.Background(),
		tts.Request{Text: "hello world", Voice: "af_heart", Speed: 1.25})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	want := speechRequest{
		Model: "hexgrad/kokoro-82m", Input: "hello world", Voice: "af_heart",
		ResponseFormat: "pcm", Speed: 1.25,
	}
	if got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
	if auth != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want the bearer key", auth)
	}
	if string(audio.Data) != "RIFFwavdata" || audio.ContentType != tts.ContentTypeWAV {
		t.Errorf("audio = %q %s, want the WAV passed through", audio.Data, audio.ContentType)
	}
}

// TestSynthesizeWrapsPCM covers OpenRouter's pcm answer: raw samples with
// the rate in the content type come back as a playable WAV.
func TestSynthesizeWrapsPCM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/pcm; rate=24000; channels=1")
		_, _ = w.Write([]byte{1, 0, 2, 0})
	}))
	defer srv.Close()

	audio, err := synthesize(t, localClient(srv.URL))
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if audio.ContentType != tts.ContentTypeWAV || !strings.HasPrefix(string(audio.Data), "RIFF") ||
		len(audio.Data) != 44+4 {
		t.Errorf("audio = %d bytes %s, want a 48-byte WAV", len(audio.Data), audio.ContentType)
	}
}

func TestSynthesizeSendsNoKeyWhenNoneIsSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want none for a keyless proxy", got)
		}
		_, _ = w.Write([]byte("RIFFwav"))
	}))
	defer srv.Close()
	if _, err := synthesize(t, localClient(srv.URL)); err != nil {
		t.Fatal(err)
	}
}

// TestSynthesizeClassifiesFailures pins the contract every surface builds
// its "why" message on: each failure is a *tts.Error whose kind says what to
// fix and whose message carries the endpoint's own reason.
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
			"bad key (check OPENROUTER_API_KEY)",
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
			"server error",
			http.StatusInternalServerError,
			`{"detail":"boom"}`,
			tts.KindUpstream,
			"openrouter returned 500: boom",
		},
		{"empty audio", http.StatusOK, "", tts.KindUpstream, "openrouter returned empty audio"},
		{"unplayable audio", http.StatusOK, "<html>", tts.KindUpstream, "audio speak cannot play"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/html")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}),
			)
			defer srv.Close()

			c := New(
				Config{Provider: "openrouter", BaseURL: srv.URL, APIKeyEnv: "OPENROUTER_API_KEY"},
			)
			_, err := synthesize(t, c)
			te, ok := errors.AsType[*tts.Error](err)
			if !ok {
				t.Fatalf("error = %v, want a *tts.Error", err)
			}
			if te.Kind != tc.wantKind || te.Provider != "openrouter" {
				t.Errorf(
					"kind = %q from %q, want %q from openrouter",
					te.Kind,
					te.Provider,
					tc.wantKind,
				)
			}
			if !strings.Contains(te.Message, tc.wantMsg) {
				t.Errorf("message = %q, want it to contain %q", te.Message, tc.wantMsg)
			}
		})
	}
}

// TestLocalFailuresPointAtTheEngine pins the local-only hints: a stopped
// engine names its t-man agent, and a failure after answering points at its
// log, while the doctor hint rides along on transport errors.
func TestLocalFailuresPointAtTheEngine(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	_, err := synthesize(t, localClient(dead.URL))
	te, ok := errors.AsType[*tts.Error](err)
	if !ok || te.Kind != tts.KindNetwork ||
		!strings.Contains(te.Message, "t-man status speak-tts") {
		t.Fatalf("error = %v, want a network error naming the speak-tts agent", err)
	}
	if !strings.Contains(err.Error(), "speak doctor") {
		t.Errorf("error = %q, want the doctor hint", err)
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000") // promise audio, then hang up
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RIFF"))
	}))
	defer broken.Close()
	_, err = synthesize(t, localClient(broken.URL))
	te, ok = errors.AsType[*tts.Error](err)
	if !ok || te.Kind != tts.KindUpstream ||
		!strings.Contains(te.Message, "broke off the audio stream") ||
		!strings.Contains(te.Message, "t-man logs speak-tts") {
		t.Fatalf("error = %v, want an upstream broken-stream error pointing at the engine log", err)
	}
}

// TestTimeoutsByEndpoint pins the timeout each endpoint gets: remote
// models answer a long part after tens of seconds, the local engine in one
// or two.
func TestTimeoutsByEndpoint(t *testing.T) {
	if got := New(Config{Local: true}).timeout; got != localTimeout {
		t.Errorf("local timeout = %s, want %s", got, localTimeout)
	}
	if got := New(Config{Provider: "openrouter"}).timeout; got != remoteTimeout {
		t.Errorf("remote timeout = %s, want %s", got, remoteTimeout)
	}
}

// stall answers nothing until the client gives up; with headers set, it
// sends the status line and a first byte of audio before it stalls.
func stall(headers bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only once the body is read does the server watch the connection,
		// so only then does the client hanging up end the request context.
		_, _ = io.Copy(io.Discard, r.Body)
		if headers {
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("R"))
			w.(http.Flusher).Flush()
		}
		<-r.Context().Done()
	}
}

// TestSlowAnswerIsNotUnreachable pins the timeout classification: an
// endpoint that took too long was reached, so the failure is upstream and
// says how long speak waited, instead of blaming the network for a slow
// model.
func TestSlowAnswerIsNotUnreachable(t *testing.T) {
	cases := []struct {
		name    string
		local   bool
		headers bool
		want    string
	}{
		{"remote, no answer", false, false, "openrouter did not answer within 50ms"},
		{"remote, stalled audio", false, true, "openrouter did not answer within 50ms"},
		{
			"local, no answer", true, false,
			"TTS engine did not answer within 50ms (check t-man logs speak-tts)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(stall(tc.headers))
			defer srv.Close()
			c := New(Config{Provider: "openrouter", Local: tc.local, BaseURL: srv.URL})
			c.timeout = shortTimeout

			_, err := synthesize(t, c)
			te, ok := errors.AsType[*tts.Error](err)
			if !ok || te.Kind != tts.KindUpstream || te.Message != tc.want {
				t.Fatalf("error = %v, want upstream %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "speak doctor") {
				t.Errorf("error = %q, want the doctor hint", err)
			}
		})
	}
}

// TestTimeoutNamesTheCallersDeadline: a caller with a shorter deadline (a
// health probe) gets a reason naming its wait, not the client's.
func TestTimeoutNamesTheCallersDeadline(t *testing.T) {
	srv := httptest.NewServer(stall(false))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	_, err := New(Config{Provider: "openrouter", BaseURL: srv.URL}).Synthesize(ctx,
		tts.Request{Text: "hello world", Voice: "af_heart"})
	const want = "openrouter did not answer within 1s"
	if te, ok := errors.AsType[*tts.Error](err); !ok || te.Kind != tts.KindUpstream ||
		te.Message != want {
		t.Fatalf("error = %v, want upstream %q", err, want)
	}
}

// TestNoConnectionIsUnreachable: only a failure to get through to the
// endpoint is a network failure, including a connect attempt that timed
// out; a caller that cancelled is not reported as a slow provider.
func TestNoConnectionIsUnreachable(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name      string
		ctx       context.Context
		transport http.RoundTripper
	}{
		{"closed port", context.Background(), nil},
		{"connect timed out", context.Background(), roundTripFunc(
			func(*http.Request) (*http.Response, error) {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}
			},
		)},
		{"caller cancelled", cancelled, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(Config{Provider: "openrouter", BaseURL: dead.URL})
			if tc.transport != nil {
				c.http.Transport = tc.transport
			}
			_, err := c.Synthesize(tc.ctx, tts.Request{Text: "hello world", Voice: "af_heart"})
			te, ok := errors.AsType[*tts.Error](err)
			if !ok || te.Kind != tts.KindNetwork ||
				!strings.Contains(te.Message, "openrouter not reachable at "+dead.URL) {
				t.Errorf("error = %v, want a network failure naming %s", err, dead.URL)
			}
		})
	}
}

// roundTripFunc stubs the transport, for failures no server can produce on
// demand.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestSynthesizeRefusesRedirects keeps the bearer key from following a
// redirect to another host.
func TestSynthesizeRefusesRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid/steal", http.StatusFound)
	}))
	defer srv.Close()
	c := New(Config{Provider: "openai", BaseURL: srv.URL, APIKey: "sk-test"})
	if _, err := synthesize(t, c); err == nil {
		t.Fatal("Synthesize followed a redirect")
	}
}

func TestReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if err := localClient(srv.URL).Reachable(context.Background()); err != nil {
		t.Errorf("Reachable = %v for a live server (any answer counts)", err)
	}
	srv.Close()
	if err := localClient(srv.URL).Reachable(context.Background()); err == nil {
		t.Error("Reachable = nil for a dead server")
	}
}
