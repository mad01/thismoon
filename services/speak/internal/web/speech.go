package web

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/services/speak/internal/audiocache"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// maxSpeechBody bounds a speech request body. One is a sentence of text plus
// a few fields; 64 KiB leaves room for a long paragraph.
const maxSpeechBody = 64 << 10

// enginezMaxAge is how long a recorded synthesis outcome answers /enginez
// before a fresh test synthesis runs. Real speech requests refresh it, so a
// page that is reading aloud never triggers a probe.
const enginezMaxAge = time.Minute

// probeTimeout bounds the /enginez test synthesis. Generous on purpose: the
// local engine's first synthesis after a restart loads the model.
const probeTimeout = 10 * time.Second

// probeText is what the /enginez test synthesis speaks: short, so a probe
// costs next to nothing on a metered provider.
const probeText = "Ready."

// Speaker is what the speech endpoints synthesize through: the active
// provider. ClipID names what decides a clip's sound besides its text and
// speed (provider, model, resolved voice), which the audio cache keys on.
type Speaker interface {
	Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error)
	ClipID(voice string) string
}

// speechRequest is the OpenAI-style body <wk-read-aloud> posts. model and
// response_format are ignored: the config picks the model, and the answer
// is always WAV or MP3.
type speechRequest struct {
	Input string  `json:"input"`
	Voice string  `json:"voice"`
	Speed float64 `json:"speed"`
}

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

// speechHandler answers POST /v1/audio/speech: answer from the audio cache,
// or synthesize through the active provider, record the outcome in health,
// store the clip, and answer audio or a speechError naming the reason.
type speechHandler struct {
	speaker Speaker
	health  *tts.Health
	store   *audiocache.Store
}

func (h *speechHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req speechRequest
	body := http.MaxBytesReader(w, r.Body, maxSpeechBody)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		http.Error(w, "speech request must be JSON with an input field: "+err.Error(),
			http.StatusBadRequest)
		return
	}
	if req.Input == "" {
		http.Error(w, "speech request has no input text", http.StatusBadRequest)
		return
	}
	key := audiocache.Key(h.speaker.ClipID(req.Voice), req.Speed, req.Input)
	if audio, ok := h.cached(key); ok {
		writeAudio(w, audio) // nothing synthesized, so nothing to record
		return
	}
	audio, err := h.speaker.Synthesize(r.Context(), tts.Request{
		Text:  req.Input,
		Voice: req.Voice,
		Speed: req.Speed,
	})
	if err != nil {
		if r.Context().Err() != nil {
			// The browser went away (navigation, closed tab); that says
			// nothing about the provider.
			return
		}
		h.fail(w, err)
		return
	}
	// Success is recorded but not emitted: read-aloud fans out one request
	// per part and would flood the event log.
	h.health.Record(nil)
	if err := h.store.Put(key, audio); err != nil {
		log.Printf("speak: cache speech: %v", err)
	}
	writeAudio(w, audio)
}

// cached returns the stored clip for key. A cache that cannot be read is
// logged and treated as a miss: the provider can still answer.
func (h *speechHandler) cached(key string) (tts.Audio, bool) {
	audio, ok, err := h.store.Get(key)
	if err != nil {
		log.Printf("speak: read speech cache: %v", err)
		return tts.Audio{}, false
	}
	return audio, ok
}

// fail records a synthesis failure, archives it as an event, and answers
// with its reason.
func (h *speechHandler) fail(w http.ResponseWriter, err error) {
	h.health.Record(err)
	emitFailure(h.health, err)
	writeFailure(w, h.health, err)
}

// classify returns err as a classified failure; an unclassified error counts
// as upstream.
func classify(health *tts.Health, err error) *tts.Error {
	if failure, ok := errors.AsType[*tts.Error](err); ok {
		return failure
	}
	return &tts.Error{
		Kind:     tts.KindUpstream,
		Provider: health.Snapshot().Provider,
		Message:  err.Error(),
	}
}

// emitFailure archives a synthesis failure as an events.this event.
func emitFailure(health *tts.Health, err error) {
	failure := classify(health, err)
	notify.EmitEvent("speak", "error", "tts synthesis failed", failure.Message,
		map[string]string{"provider": health.Snapshot().Provider, "kind": string(failure.Kind)})
}

// writeFailure answers a failed synthesis with its reason and the health
// state it left behind. It records nothing: the caller already has.
func writeFailure(w http.ResponseWriter, health *tts.Health, err error) {
	failure := classify(health, err)
	state := health.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(failureStatus(failure.Kind))
	body := speechError{Error: speechErrorDetail{
		Message:  failure.Message,
		Type:     failure.Kind,
		Provider: state.Provider,
		Model:    state.Model,
		Health:   state.Status,
	}}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("speak: encode speech error: %v", err)
	}
}

// writeAudio answers a clip.
func writeAudio(w http.ResponseWriter, audio tts.Audio) {
	w.Header().Set("Content-Type", audio.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(audio.Data)))
	_, _ = w.Write(audio.Data)
}

// failureStatus is the status speak answers a failed synthesis with. speak is
// a gateway to the provider, so a provider's own 401 or 404 is a 502 here:
// the browser's request was fine. A rate limit stays 429 so clients know to
// retry later, and a config problem is speak's own 503.
func failureStatus(kind tts.Kind) int {
	switch kind {
	case tts.KindQuota:
		return http.StatusTooManyRequests
	case tts.KindConfig:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

// enginez answers GET /enginez with the health state, running a test
// synthesis first when nothing has been recorded for enginezMaxAge. A ping
// alone would call a running engine with a missing model healthy.
type enginez struct {
	speaker Speaker
	health  *tts.Health

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
	_, err := e.speaker.Synthesize(probeCtx, tts.Request{Text: probeText})
	if err != nil && errors.Is(ctx.Err(), context.Canceled) {
		return // the caller left mid-probe; the outcome says nothing about the provider
	}
	e.health.Record(err)
}
