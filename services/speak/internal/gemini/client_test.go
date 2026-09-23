package gemini

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/speak/internal/tts"
)

const model = "gemini-3.1-flash-tts-preview"

// pcm is two 16-bit samples, what an audio answer carries base64-encoded.
var pcm = []byte{1, 0, 2, 0}

// audioAnswer is a generateContent answer carrying pcm as L16 at rate.
func audioAnswer(rate string) string {
	return `{"candidates":[{"content":{"parts":[{"inlineData":{
		"mimeType":"audio/L16;codec=pcm;rate=` + rate + `",
		"data":"` + base64.StdEncoding.EncodeToString(pcm) + `"}}]},
		"finishReason":"STOP"}]}`
}

// testClient is a client for a stand-in API at url that retries without
// waiting.
func testClient(url string) *Client {
	c := New(Config{
		Provider:  "gemini",
		BaseURL:   url,
		APIKey:    "g-key",
		APIKeyEnv: "GEMINI_API_KEY",
		Model:     model,
	})
	c.retryDelay = 0
	return c
}

func synthesize(c *Client) (tts.Audio, error) {
	return c.Synthesize(context.Background(), tts.Request{Text: "hello world", Voice: "Puck"})
}

// TestSynthesizeSendsTheGenerateContentRequest pins the request: the model's
// generateContent path, the key in its header and never the URL, the text
// as the one part, audio as the only modality, the voice by name. The
// answer's L16 samples come back as a WAV at the rate the answer names.
func TestSynthesizeSendsTheGenerateContentRequest(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost ||
			r.URL.Path != "/v1beta/models/"+model+":generateContent" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("query = %q, want the key kept out of the URL", r.URL.RawQuery)
		}
		if key := r.Header.Get("x-goog-api-key"); key != "g-key" {
			t.Errorf("x-goog-api-key = %q, want the key", key)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(audioAnswer("16000")))
	}))
	defer srv.Close()

	// A base URL ending in /v1beta and a models/-prefixed id both normalize.
	c := New(Config{
		Provider: "gemini", BaseURL: srv.URL + "/v1beta/", APIKey: "g-key",
		Model: "models/" + model,
	})
	audio, err := c.Synthesize(context.Background(),
		tts.Request{Text: "hello world", Voice: "Puck", Speed: 1.5})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	var want map[string]any
	_ = json.Unmarshal([]byte(`{
		"contents":[{"parts":[{"text":"hello world"}]}],
		"generationConfig":{"responseModalities":["AUDIO"],
			"speechConfig":{"voiceConfig":{"prebuiltVoiceConfig":{"voiceName":"Puck"}}}}}`), &want)
	if gotJSON, wantJSON := mustJSON(t, got), mustJSON(t, want); gotJSON != wantJSON {
		t.Errorf("request = %s\nwant %s", gotJSON, wantJSON)
	}
	if audio.ContentType != tts.ContentTypeWAV || !strings.HasPrefix(string(audio.Data), "RIFF") ||
		len(audio.Data) != 44+len(pcm) {
		t.Fatalf(
			"audio = %s %d bytes, want the samples in a WAV",
			audio.ContentType,
			len(audio.Data),
		)
	}
	if rate := binary.LittleEndian.Uint32(audio.Data[24:28]); rate != 16000 {
		t.Errorf("WAV sample rate = %d, want the answer's 16000", rate)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestFailuresAreClassified pins what each Google answer becomes, and which
// ones are retried: server errors and answers without audio get exactly one
// more try, everything else none.
func TestFailuresAreClassified(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		kind     tts.Kind
		reason   string
		attempts int32
	}{
		{
			"bad key", 400,
			`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.",
			"status":"INVALID_ARGUMENT","details":[{"reason":"API_KEY_INVALID"}]}}`,
			tts.KindAuth, "API key not valid. Please pass a valid API key. (check GEMINI_API_KEY)", 1,
		},
		{
			"key not allowed", 403,
			`{"error":{"code":403,"message":"Method doesn't allow unregistered callers.","status":"PERMISSION_DENIED"}}`,
			tts.KindAuth, "(check GEMINI_API_KEY)", 1,
		},
		{
			"quota", 429,
			`{"error":{"code":429,"message":"You exceeded your current quota.","status":"RESOURCE_EXHAUSTED"}}`,
			tts.KindQuota, "gemini returned 429: You exceeded your current quota.", 1,
		},
		{
			"no such model", 404,
			`{"error":{"code":404,"message":"models/nope is not found","status":"NOT_FOUND"}}`,
			tts.KindModel, "models/nope is not found", 1,
		},
		{
			"bad voice", 400,
			`{"error":{"code":400,"message":"Voice name Nope is not supported.","status":"INVALID_ARGUMENT"}}`,
			tts.KindUpstream, "gemini returned 400: Voice name Nope is not supported.", 1,
		},
		{
			"server error", 500,
			`{"error":{"code":500,"message":"Internal error encountered.","status":"INTERNAL"}}`,
			tts.KindUpstream, "gemini returned 500: Internal error encountered.", 2,
		},
		{
			"blocked", 200,
			`{"promptFeedback":{"blockReason":"PROHIBITED_CONTENT"}}`,
			tts.KindUpstream, "gemini refused the text (PROHIBITED_CONTENT)", 1,
		},
		{
			"no audio", 200,
			`{"candidates":[{"content":{"parts":[{"text":"I can't read that."}]},"finishReason":"OTHER"}]}`,
			tts.KindUpstream, "gemini returned no audio (finish reason OTHER)", 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					attempts.Add(1)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}),
			)
			defer srv.Close()

			_, err := synthesize(testClient(srv.URL))
			te, ok := errors.AsType[*tts.Error](err)
			if !ok {
				t.Fatalf("err = %v (%T), want a *tts.Error", err, err)
			}
			if te.Kind != tc.kind || te.Provider != "gemini" ||
				!strings.Contains(te.Message, tc.reason) {
				t.Errorf(
					"err = %s %q, want %s containing %q",
					te.Kind,
					te.Message,
					tc.kind,
					tc.reason,
				)
			}
			if n := attempts.Load(); n != tc.attempts {
				t.Errorf("attempts = %d, want %d", n, tc.attempts)
			}
		})
	}
}

func TestOneRetryRecoversARandomFailure(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(audioAnswer("24000")))
	}))
	defer srv.Close()

	audio, err := synthesize(testClient(srv.URL))
	if err != nil || audio.ContentType != tts.ContentTypeWAV {
		t.Fatalf("Synthesize = %s, %v; want audio from the retry", audio.ContentType, err)
	}
}

// roundTripFunc stubs the transport, for answers no server needs to send.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestRetryStopsWithTheContext: a request cancelled during the wait before
// the retry returns the first failure instead of waiting it out.
func TestRetryStopsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var attempts atomic.Int32
	c := testClient("http://gemini.invalid")
	c.retryDelay = time.Hour
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts.Add(1)
		cancel() // the caller gives up while the first answer is on its way
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    r,
		}, nil
	})

	done := make(chan error, 1)
	go func() {
		_, err := c.Synthesize(ctx, tts.Request{Text: "hi", Voice: "Kore"})
		done <- err
	}()
	select {
	case err := <-done:
		te, ok := errors.AsType[*tts.Error](err)
		if !ok || te.Status != http.StatusServiceUnavailable || attempts.Load() != 1 {
			t.Errorf(
				"err = %v after %d attempts, want the first attempt's 503",
				err,
				attempts.Load(),
			)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Synthesize waited out the retry delay after the context ended")
	}
}

func TestUnreachableIsANetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	_, err := synthesize(testClient(url))
	te, ok := errors.AsType[*tts.Error](err)
	if !ok || te.Kind != tts.KindNetwork || te.Status != 0 ||
		!strings.Contains(te.Message, "not reachable at "+url) {
		t.Errorf("err = %v, want a network failure naming %s", err, url)
	}
}
