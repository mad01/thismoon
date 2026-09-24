package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/speak/internal/config"
	"github.com/mad01/thismoon/services/speak/internal/provider"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// testInfo is the build metadata the test mux reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test-sha",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "speak/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

// testProvider is a local provider whose engine is at engineURL. It curates
// its voices, so no test reads the real Hugging Face cache.
func testProvider(engineURL string) *provider.Provider {
	return provider.New(context.Background(), config.Provider{
		Name:    "local",
		Type:    config.TypeLocal,
		BaseURL: engineURL,
		Model:   "mlx-community/Kokoro-82M-bf16",
		Voice:   "af_heart",
		Voices:  []string{"af_heart", "am_adam"},
		Format:  "wav",
	})
}

// newTestMux serves the page against testProvider(engineURL).
func newTestMux(t *testing.T, engineURL string) *http.ServeMux {
	t.Helper()
	return muxFor(t, testProvider(engineURL))
}

// configFor serves p with an audio cache of its own, so no test answers from
// another's clips.
func configFor(t *testing.T, p *provider.Provider) Config {
	t.Helper()
	return Config{Speaker: p, Health: p.NewHealth(), Info: testInfo, CacheDir: t.TempDir()}
}

// muxFor is the page's mux against p; handler wraps it in what Serve adds.
func muxFor(t *testing.T, p *provider.Provider) *http.ServeMux {
	t.Helper()
	return NewMux(configFor(t, p))
}

// TestVersion pins the cross-tool build metadata contract: the four keys, the
// injected values, and the headers ralph and status probe with.
func TestVersion(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var keys map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatalf("unmarshal %q: %v", rec.Body.Bytes(), err)
	}
	want := map[string]string{
		"version":    testInfo.Version,
		"commit":     testInfo.Commit,
		"tag":        testInfo.Tag,
		"build_time": testInfo.BuildTime,
	}
	if len(keys) != len(want) {
		t.Errorf("version body = %q, want exactly the keys %v", rec.Body.Bytes(), want)
	}
	for k, v := range want {
		if keys[k] != v {
			t.Errorf("version[%q] = %q, want %q", k, keys[k], v)
		}
	}
}

func TestIndexServesShell(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	// The shell is chrome only: header + a mount point + the webkit/app scripts.
	// The upload form and <wk-read-aloud> are built client-side by app.js now.
	for _, want := range []string{`id="app"`, "/app.js", "/webkit/webkit.js", "wk-header"} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
}

func TestAppJSServesJavaScript(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /app.js: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("GET /app.js Content-Type = %q, want application/javascript", ct)
	}
	body := rec.Body.String()
	// The client logic that left the shell: the upload form posts to /read and
	// builds <wk-read-aloud> over the rendered sections.
	for _, want := range []string{"/read", "wk-read-aloud"} {
		if !strings.Contains(body, want) {
			t.Errorf("app.js missing %q", want)
		}
	}
}

func TestReadReturnsRenderedJSON(t *testing.T) {
	mux := newTestMux(t, "http://127.0.0.1:1")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("doc", "notes.md")
	_, _ = fw.Write([]byte("## Hello\n\nworld paragraph\n"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/read", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /read: status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST /read Content-Type = %q, want application/json", ct)
	}
	var got readResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /read response: %v (body: %s)", err, rec.Body.String())
	}
	if got.Name != "notes.md" {
		t.Errorf("name = %q, want notes.md", got.Name)
	}
	for _, want := range []string{
		`<section class="doc-section" data-section="1" data-ra-parts=`, "Hello</h2>",
		"world paragraph",
	} {
		if !strings.Contains(got.Content, want) {
			t.Errorf("rendered content missing %q", want)
		}
	}
}

// TestSpeechAnswersAudioWithCORS pins the success path: the request goes to
// the provider's endpoint and the audio comes back with exactly one CORS
// header, ours (the engine's own must never leak through).
func TestSpeechAnswersAudioWithCORS(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("upstream path = %q, want /v1/audio/speech", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(b), "af_heart") {
			t.Errorf("upstream body = %q, want voice payload", b)
		}
		// Real mlx-audio reflects the request origin; the proxy must strip
		// this or the response carries two ACAO values and browsers reject it.
		w.Header().Set("Access-Control-Allow-Origin", "http://present.this")
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFfake"))
	}))
	defer upstream.Close()

	mux := newTestMux(t, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"af_heart"}`))
	req.Header.Set("Origin", "http://present.this")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("speech status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Values("Access-Control-Allow-Origin"); len(got) != 1 ||
		got[0] != "http://present.this" {
		t.Errorf("ACAO values = %v, want exactly [http://present.this]", got)
	}
	if rec.Body.String() != "RIFFfake" {
		t.Errorf("body = %q, want upstream audio passthrough", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != tts.ContentTypeWAV {
		t.Errorf("Content-Type = %q, want %s", ct, tts.ContentTypeWAV)
	}
}

// TestSpeechAnswersRepeatsFromTheCache pins the read-through cache: the same
// text, voice and speed are synthesized once, and a different speed is a
// different clip.
func TestSpeechAnswersRepeatsFromTheCache(t *testing.T) {
	engine := newFakeEngine(t, http.StatusOK, "RIFFfake")
	mux := newTestMux(t, engine.URL)
	for i, body := range []string{
		`{"input":"hi","voice":"af_heart"}`,
		`{"input":"hi","voice":"af_heart"}`,
		`{"input":"hi","voice":"Kore"}`, // resolves to af_heart: the same clip
		`{"input":"hi","voice":"af_heart","speed":1.5}`,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
			strings.NewReader(body)))
		if rec.Code != http.StatusOK || rec.Body.String() != "RIFFfake" {
			t.Fatalf("request %d = %d %q, want the clip", i+1, rec.Code, rec.Body.String())
		}
	}
	if n := engine.requests.Load(); n != 2 {
		t.Errorf("engine saw %d requests, want 2 (one per distinct clip)", n)
	}
}

// TestSpeechMapsUnknownVoiceToDefault covers the read-aloud component on a
// non-Kokoro provider: it sends af_heart everywhere, and a voice the
// provider does not offer becomes the provider's default.
func TestSpeechMapsUnknownVoiceToDefault(t *testing.T) {
	var gotVoice string
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Voice string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotVoice = body.Voice
		_, _ = w.Write([]byte("RIFFfake"))
	}))
	defer engine.Close()

	rec := httptest.NewRecorder()
	newTestMux(
		t,
		engine.URL,
	).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"Kore"}`)))
	if rec.Code != http.StatusOK || gotVoice != "af_heart" {
		t.Errorf(
			"status %d, provider got voice %q; want 200 with the default af_heart",
			rec.Code,
			gotVoice,
		)
	}
}

// TestSpeechRejectsBadRequestsWithoutBlamingTheProvider pins that a
// malformed request is the caller's fault: 400, and health stays unknown.
func TestSpeechRejectsBadRequestsWithoutBlamingTheProvider(t *testing.T) {
	engine := newFakeEngine(t, http.StatusOK, "RIFFfake")
	mux := newTestMux(t, engine.URL)
	for _, body := range []string{`not json`, `{"voice":"af_heart"}`} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(
			rec,
			httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(body)),
		)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	if n := engine.requests.Load(); n != 0 {
		t.Errorf("engine saw %d requests, want none for bad requests", n)
	}
}

// TestSpeechConfigProblemIs503 covers a provider that cannot be built: every
// request answers 503 with the config reason, and /enginez reports it
// without probing anything.
func TestSpeechConfigProblemIs503(t *testing.T) {
	mux := muxFor(t, provider.New(context.Background(), config.Provider{
		Name: "openrouter", Type: config.TypeOpenRouter, Problem: "OPENROUTER_API_KEY is not set",
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi"}`)))
	var body speechError
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusServiceUnavailable || body.Error.Type != tts.KindConfig ||
		!strings.Contains(body.Error.Message, "OPENROUTER_API_KEY is not set") {
		t.Errorf("speech = %d %+v, want 503 naming the missing key", rec.Code, body.Error)
	}
	_, state := getEnginez(t, mux)
	if state.Status != tts.StatusDown || state.Kind != tts.KindConfig {
		t.Errorf("enginez = %+v, want down with kind config", state)
	}
}

// fakeEngine is a TTS engine stand-in that answers every speech request with
// status and body, counting the requests so tests can tell a probe from a
// cached answer.
type fakeEngine struct {
	*httptest.Server
	requests atomic.Int32
}

func newFakeEngine(t *testing.T, status int, body string) *fakeEngine {
	t.Helper()
	fe := &fakeEngine{}
	fe.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fe.requests.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fe.Close)
	return fe
}

func getEnginez(t *testing.T, mux *http.ServeMux) (int, tts.State) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/enginez", nil))
	var state tts.State
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode enginez %q: %v", rec.Body.String(), err)
	}
	return rec.Code, state
}

// TestEnginezProbesSynthesis pins that /enginez answers from a real test
// synthesis, not a ping, and reuses a fresh outcome instead of probing on
// every poll.
func TestEnginezProbesSynthesis(t *testing.T) {
	engine := newFakeEngine(t, http.StatusOK, "RIFFfake")
	mux := newTestMux(t, engine.URL)

	code, state := getEnginez(t, mux)
	if code != http.StatusOK || state.Status != tts.StatusOK || state.Provider != "local" {
		t.Errorf("enginez = %d %+v, want 200 ok from provider local", code, state)
	}
	getEnginez(t, mux)
	if n := engine.requests.Load(); n != 1 {
		t.Errorf(
			"engine saw %d synthesis requests, want 1 (second poll reuses the fresh outcome)",
			n,
		)
	}
}

// TestEnginezNamesTheFailure covers the states a reachability ping reported
// as healthy or as a bare 502: each comes back as 503 with the reason.
func TestEnginezNamesTheFailure(t *testing.T) {
	cases := []struct {
		name       string
		engineURL  func(t *testing.T) string
		wantStatus tts.Status
		wantKind   tts.Kind
		wantReason string
	}{
		{
			name: "engine up, model missing",
			engineURL: func(t *testing.T) string {
				return newFakeEngine(t, http.StatusInternalServerError,
					`{"detail":"LocalEntryNotFoundError: Kokoro-82M"}`).URL
			},
			wantStatus: tts.StatusDegraded,
			wantKind:   tts.KindUpstream,
			wantReason: "LocalEntryNotFoundError: Kokoro-82M",
		},
		{
			name: "engine stopped",
			engineURL: func(t *testing.T) string {
				fe := newFakeEngine(t, http.StatusOK, "")
				fe.Close()
				return fe.URL
			},
			wantStatus: tts.StatusDown,
			wantKind:   tts.KindNetwork,
			wantReason: "not reachable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, state := getEnginez(t, newTestMux(t, tc.engineURL(t)))
			if code != http.StatusServiceUnavailable {
				t.Errorf("enginez status = %d, want 503", code)
			}
			if state.Status != tc.wantStatus || state.Kind != tc.wantKind {
				t.Errorf(
					"state = %s/%s, want %s/%s",
					state.Status,
					state.Kind,
					tc.wantStatus,
					tc.wantKind,
				)
			}
			if !strings.Contains(state.Reason, tc.wantReason) {
				t.Errorf("reason = %q, want it to contain %q", state.Reason, tc.wantReason)
			}
		})
	}
}

// TestSpeechFailureCarriesTheReason pins the body <wk-read-aloud> shows: a
// failed synthesis answers JSON naming the cause, keeps the CORS header so a
// sibling page can read it, and feeds the same health /enginez reports.
func TestSpeechFailureCarriesTheReason(t *testing.T) {
	engine := newFakeEngine(
		t,
		http.StatusInternalServerError,
		`{"detail":"voice xx_nope not found"}`,
	)
	mux := newTestMux(t, engine.URL)

	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"xx_nope"}`))
	req.Header.Set("Origin", "http://present.this")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf(
			"status = %d, want 502 (speak is the gateway; the browser's request was fine)",
			rec.Code,
		)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://present.this" {
		t.Errorf("ACAO = %q, want the allowlisted origin", got)
	}
	var body speechError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	if !strings.Contains(body.Error.Message, "voice xx_nope not found") ||
		body.Error.Provider != "local" || body.Error.Health != tts.StatusDegraded {
		t.Errorf(
			"error body = %+v, want the engine's reason, provider local, health degraded",
			body.Error,
		)
	}

	_, state := getEnginez(t, mux)
	if !strings.Contains(state.Reason, "voice xx_nope not found") {
		t.Errorf("enginez reason = %q, want the failure the real request recorded", state.Reason)
	}
	if n := engine.requests.Load(); n != 1 {
		t.Errorf("engine saw %d requests, want 1 (enginez answers from the recorded failure)", n)
	}
}

// TestSpeechEmptyAudioCountsAsFailure pins the case the status line hides:
// mlx-audio answers 200 and then fails mid-stream, sending no audio. The
// browser gets a 502 naming it, and /enginez reports it without a probe.
func TestSpeechEmptyAudioCountsAsFailure(t *testing.T) {
	engine := newFakeEngine(t, http.StatusOK, "")
	mux := newTestMux(t, engine.URL)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"af_heart"}`)))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "empty audio") {
		t.Fatalf("speech = %d %q, want 502 naming the empty audio", rec.Code, rec.Body.String())
	}

	_, state := getEnginez(t, mux)
	if state.Status != tts.StatusDegraded || state.Kind != tts.KindUpstream ||
		!strings.Contains(state.Reason, "empty audio") {
		t.Errorf("state = %+v, want degraded with the empty-audio reason", state)
	}
	if n := engine.requests.Load(); n != 1 {
		t.Errorf("engine saw %d requests, want 1 (enginez answers from the recorded failure)", n)
	}
}

// TestSpeechBrokenStreamIsUpstream pins the failure this machine's engine
// showed live: 200, then the stream breaks off. The engine is reachable, so
// it must read as upstream, never network.
func TestSpeechBrokenStreamIsUpstream(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000") // promise audio, then hang up
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RIFF"))
	}))
	defer engine.Close()
	mux := newTestMux(t, engine.URL)

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"af_heart"}`)))

	_, state := getEnginez(t, mux)
	if state.Kind != tts.KindUpstream ||
		!strings.Contains(state.Reason, "broke off the audio stream") {
		t.Errorf("state = %+v, want an upstream broken-stream failure", state)
	}
}

func TestSpeechUnreachableEngineIsNamed(t *testing.T) {
	engine := newFakeEngine(t, http.StatusOK, "")
	engine.Close()
	mux := newTestMux(t, engine.URL)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		strings.NewReader(`{"input":"hi","voice":"af_heart"}`)))

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
	var body speechError
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Type != tts.KindNetwork ||
		!strings.Contains(body.Error.Message, "not reachable") {
		t.Errorf("error body = %+v, want a network failure naming the engine", body.Error)
	}
}

// TestSpeechPreflightAnsweredLocally pins that the served handler answers a
// preflight itself: nothing reaches the engine.
func TestSpeechPreflightAnsweredLocally(t *testing.T) {
	// Upstream that fails the test if the preflight is forwarded.
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("OPTIONS preflight must not reach the upstream")
	}))
	defer upstream.Close()

	h := handler(configFor(t, testProvider(upstream.URL)))
	req := httptest.NewRequest(http.MethodOptions, "/v1/audio/speech", nil)
	req.Header.Set("Origin", "http://present.this")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("Allow-Methods = %q, want POST", got)
	}
}
