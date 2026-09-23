package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/tts"
	"github.com/mad01/thismoon/services/speak/internal/ttsclient"
)

// maxErrorBody bounds how much of an engine error response is read for its
// message before the body is replaced with speak's own.
const maxErrorBody = 4 << 10

// enginezMaxAge is how long a recorded synthesis outcome answers /enginez
// before a fresh test synthesis runs. Real speech requests refresh it, so a
// page that is reading aloud never triggers a probe.
const enginezMaxAge = time.Minute

// probeTimeout bounds the /enginez test synthesis. Generous on purpose: the
// engine's first synthesis after a restart loads the model.
const probeTimeout = 10 * time.Second

// probeText is what the /enginez test synthesis speaks: short, so a probe
// costs next to nothing on a metered backend.
const probeText = "Ready."

// speechError is the body of a failed POST /v1/audio/speech, in the OpenAI
// error shape ({"error": {"message", "type"}}) plus which provider failed and
// the health state the failure left behind. <wk-read-aloud> shows Message.
type speechError struct {
	Error speechErrorDetail `json:"error"`
}

type speechErrorDetail struct {
	Message  string     `json:"message"`
	Type     tts.Kind   `json:"type"`
	Provider string     `json:"provider"`
	Model    string     `json:"model,omitempty"`
	Health   tts.Status `json:"health"`
}

// newSpeechProxy fronts the engine's /v1/audio/speech. A good answer passes
// through untouched; a failed one is replaced by a speechError body naming
// the reason; every outcome is recorded in health.
func newSpeechProxy(upstream *url.URL, health *tts.Health) *httputil.ReverseProxy {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			// Let the transport negotiate compression itself, so an error
			// body arrives decoded and can be read for its message.
			pr.Out.Header.Del("Accept-Encoding")
		},
	}
	proxy.ModifyResponse = func(res *http.Response) error {
		// The engine sets its own CORS headers; ours are already on the
		// response, and duplicated Access-Control-Allow-Origin values make
		// browsers reject the response outright. Strip the upstream's set.
		for _, h := range []string{
			"Access-Control-Allow-Origin", "Access-Control-Allow-Methods",
			"Access-Control-Allow-Headers", "Access-Control-Allow-Credentials",
			"Access-Control-Max-Age",
		} {
			res.Header.Del(h)
		}
		if res.StatusCode < http.StatusBadRequest {
			// Recorded when the body ends, not now: the engine can fail
			// after answering 200 and send no audio at all. Success is
			// recorded but not emitted: read-aloud fans out one request per
			// sentence and would flood the event log.
			res.Body = &recordingBody{ReadCloser: res.Body, done: func(n int64, err error) {
				if err == nil && n > 0 {
					health.Record(nil)
					return
				}
				failure := ttsclient.FailedAfterAnswerError(res.StatusCode, err)
				health.Record(failure)
				notify.EmitEvent("speak", "error", "tts synthesis failed", failure.Message, nil)
			}}
			return nil
		}
		detail, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		_ = res.Body.Close()
		failure := tts.FromResponse(ttsclient.Provider, "TTS engine", res.StatusCode, detail)
		health.Record(failure)
		notify.EmitEvent("speak", "error", "tts synthesis failed", failure.Message,
			map[string]string{
				"status": strconv.Itoa(res.StatusCode),
				"kind":   string(failure.Kind),
			})
		body := encodeSpeechError(failure, health.Snapshot())
		res.Body = io.NopCloser(bytes.NewReader(body))
		res.ContentLength = int64(len(body))
		res.Header.Set("Content-Length", strconv.Itoa(len(body)))
		res.Header.Set("Content-Type", "application/json")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Context().Err() != nil {
			// The browser went away (navigation, closed tab); that says
			// nothing about the engine.
			return
		}
		failure := ttsclient.UnreachableError(upstream.String(), err)
		health.Record(failure)
		notify.EmitEvent("speak", "error", "tts engine unreachable", err.Error(), nil)
		writeSpeechError(w, http.StatusBadGateway, failure, health.Snapshot())
	}
	return proxy
}

// recordingBody passes a streamed audio body through, counting its bytes,
// and calls done once: at EOF with the total, or with the error when the
// engine breaks off the stream. A body closed early (the browser left
// mid-clip) reports only if it carried audio: no bytes before an early close
// says nothing about the engine.
type recordingBody struct {
	io.ReadCloser
	n        int64
	reported bool
	done     func(n int64, err error)
}

func (b *recordingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	switch {
	case errors.Is(err, io.EOF):
		b.report(nil)
	case err != nil:
		b.report(err)
	}
	return n, err
}

func (b *recordingBody) Close() error {
	if b.n > 0 {
		b.report(nil)
	}
	return b.ReadCloser.Close()
}

func (b *recordingBody) report(err error) {
	if b.reported {
		return
	}
	b.reported = true
	b.done(b.n, err)
}

// encodeSpeechError renders the speechError body for a classified failure.
func encodeSpeechError(failure *tts.Error, state tts.State) []byte {
	body, err := json.Marshal(speechError{Error: speechErrorDetail{
		Message:  failure.Message,
		Type:     failure.Kind,
		Provider: failure.Provider,
		Model:    state.Model,
		Health:   state.Status,
	}})
	if err != nil {
		// Every field is a string; Marshal cannot fail on this shape.
		panic("web: encode speech error: " + err.Error())
	}
	return body
}

func writeSpeechError(w http.ResponseWriter, status int, failure *tts.Error, state tts.State) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(encodeSpeechError(failure, state))
}

// enginez answers GET /enginez with the health state, running a test
// synthesis first when nothing has been recorded for enginezMaxAge. A ping
// alone would call a running engine with a missing model healthy.
type enginez struct {
	engine *ttsclient.Client
	health *tts.Health

	probeMu sync.Mutex // one probe at a time; callers queued behind it reuse its result
}

func (e *enginez) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCORS(w, r)
	e.refresh(r.Context())
	state := e.health.Snapshot()
	status := http.StatusOK
	if state.Status != tts.StatusOK {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(state); err != nil {
		log.Printf("speak: encode enginez state: %v", err)
	}
}

// refresh runs a test synthesis unless a fresh outcome is already recorded.
func (e *enginez) refresh(ctx context.Context) {
	e.probeMu.Lock()
	defer e.probeMu.Unlock()
	if e.health.CheckedWithin(enginezMaxAge) {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	_, err := e.engine.Synthesize(probeCtx, probeText, speak.DefaultVoice)
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		return // the caller left mid-probe; the outcome says nothing about the engine
	}
	e.health.Record(err)
}
