// Package ttsclient is a thin HTTP client for the local Kokoro TTS engine
// (mlx-audio) that `speak serve` also reverse-proxies. The MCP playback engine
// fetches per-sentence WAV audio from it and plays the bytes with afplay —
// server-side synthesis stays in the engine, this package only speaks HTTP.
// Every failure comes back as a *tts.Error, so callers can say why speech
// failed rather than just that it did.
package ttsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// Provider is the name the local engine reports under in health state and
// error bodies.
const Provider = "local"

// maxErrorBody bounds how much of an engine error response is read for its
// message.
const maxErrorBody = 4 << 10

// Client talks to the mlx-audio engine's OpenAI-compatible speech endpoint.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the engine at baseURL (e.g. http://127.0.0.1:8765).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// BaseURL is the engine address the client was built for.
func (c *Client) BaseURL() string { return c.baseURL }

// speechRequest is the /v1/audio/speech payload. model is required: the
// engine answers 422 without it. response_format is always "wav": the
// engine's default mp3 path shells out to ffmpeg, which may be missing and
// then fails as a 200 with an empty body rather than an error.
type speechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// Synthesize returns WAV bytes for one chunk of text in the given voice. A
// failure is a *tts.Error: network when the engine never answered, otherwise
// classified from the engine's status and message.
func (c *Client) Synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	body, err := json.Marshal(speechRequest{
		Model:          speak.DefaultModel,
		Input:          text,
		Voice:          voice,
		ResponseFormat: "wav",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal speech request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/v1/audio/speech",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build speech request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		// The one transport chokepoint every playback path goes through;
		// Hint points the reader at the operating doc from here.
		return nil, agentdoc.Hint(UnreachableError(c.baseURL, err), speak.Facts())
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		return nil, tts.FromResponse(Provider, "TTS engine", res.StatusCode, detail)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil || len(data) == 0 {
		return nil, FailedAfterAnswerError(res.StatusCode, err)
	}
	return data, nil
}

// FailedAfterAnswerError is the classified failure for an engine that
// answered 200 and then sent no audio, or broke off the stream (err set). It
// hit an error after the status line was already out, so the cause is only
// in its own log. It counts as upstream, not network: the engine is
// reachable. The web proxy detects the same case at the end of the body.
func FailedAfterAnswerError(status int, err error) *tts.Error {
	what := "returned empty audio"
	if err != nil {
		what = "broke off the audio stream (" + err.Error() + ")"
	}
	return &tts.Error{
		Kind:     tts.KindUpstream,
		Provider: Provider,
		Status:   status,
		Message: "TTS engine " + what + " after answering, " +
			"so the cause is in its log (t-man logs speak-tts)",
		Err: err,
	}
}

// UnreachableError is the classified failure for an engine that never
// answered. The web proxy reports its transport errors through it too, so
// both surfaces word a stopped engine the same way.
func UnreachableError(baseURL string, err error) *tts.Error {
	return &tts.Error{
		Kind:     tts.KindNetwork,
		Provider: Provider,
		Message: fmt.Sprintf(
			"TTS engine not reachable at %s (is the speak-tts agent running? t-man status speak-tts): %v",
			baseURL,
			err,
		),
		Err: err,
	}
}

// Reachable reports whether the engine answers a quick GET / within 1.5s: a
// cheap ping for speak_status. It proves a process is listening, not that it
// can synthesize; the recorded tts.Health answers that.
func (c *Client) Reachable() bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	res, err := client.Get(c.baseURL + "/")
	if err != nil {
		return false
	}
	_ = res.Body.Close()
	return res.StatusCode < 500
}
