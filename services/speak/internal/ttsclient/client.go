// Package ttsclient is the HTTP client for OpenAI-compatible speech
// endpoints (POST {base}/v1/audio/speech): the local Kokoro engine
// (mlx-audio), OpenRouter, OpenAI and a LiteLLM proxy all serve that one
// surface and differ only in base URL, key, model and the audio format they
// return. Every answer is normalized to playable WAV or MP3, and every
// failure comes back as a *tts.Error, so callers can say why speech failed
// rather than just that it did.
package ttsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// maxErrorBody bounds how much of an error response is read for its
// message.
const maxErrorBody = 4 << 10

// localTimeout bounds one synthesis on the local engine, where a part takes
// a second or two; this only stops a hung engine from holding a play click
// forever.
const localTimeout = 30 * time.Second

// remoteTimeout bounds one synthesis on a remote provider. Remote speech
// models answer with the whole clip at once, after 20 seconds and more for a
// part of a few hundred characters, and now and then took longer than 30.
const remoteTimeout = 2 * time.Minute

// errRedirectRefused stops the client before it can re-send the request, and
// its Authorization header, to a redirect target.
var errRedirectRefused = errors.New("ttsclient: HTTP redirect refused")

// Config locates one OpenAI-compatible speech endpoint.
type Config struct {
	Provider  string // the config block's name, reported in health and errors
	Local     bool   // the local mlx-audio engine: failures point at its t-man agent
	BaseURL   string // endpoint root, without /v1
	APIKey    string // sent as a bearer token when set
	APIKeyEnv string // named in the reason when the key is refused
	Model     string
	Format    string // response_format to request: wav, or pcm where wav is not offered
}

// Client synthesizes speech against one endpoint.
type Client struct {
	cfg     Config
	http    *http.Client
	timeout time.Duration // bounds one synthesis: localTimeout or remoteTimeout
}

// New returns a Client for cfg.
func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	timeout := remoteTimeout
	if cfg.Local {
		timeout = localTimeout
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirectRefused
			},
		},
		timeout: timeout,
	}
}

// BaseURL is the endpoint root the client was built for.
func (c *Client) BaseURL() string { return c.cfg.BaseURL }

// speechRequest is the /v1/audio/speech payload. model is required: the
// local engine answers 422 without it. response_format is wav wherever it
// is offered: the local engine's default mp3 path shells out to ffmpeg,
// which may be missing and then fails as a 200 with an empty body.
type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed,omitempty"`
}

// Synthesize returns playable audio for one chunk of text. A failure is a
// *tts.Error: network when the endpoint could not be reached, upstream when
// it did not answer within the timeout, otherwise classified from its status
// and message.
func (c *Client) Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error) {
	ctx, cancel, limit := tts.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := c.newRequest(ctx, req)
	if err != nil {
		return tts.Audio{}, err
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		// The one transport chokepoint every playback path goes through;
		// Hint points the reader at the operating doc from here.
		return tts.Audio{}, agentdoc.Hint(c.unanswered(limit, err), speak.Facts())
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		failure := tts.FromResponse(c.cfg.Provider, c.label(), res.StatusCode, detail)
		if failure.Kind == tts.KindAuth && c.cfg.APIKeyEnv != "" {
			failure.Message += " (check " + c.cfg.APIKeyEnv + ")"
		}
		return tts.Audio{}, failure
	}
	data, err := io.ReadAll(res.Body)
	if tts.TimedOut(err) {
		return tts.Audio{}, agentdoc.Hint(c.slow(limit, err), speak.Facts())
	}
	if err != nil || len(data) == 0 {
		return tts.Audio{}, c.failedAfterAnswer(res.StatusCode, err)
	}
	audio, err := tts.Normalize(data, res.Header.Get("Content-Type"))
	if err != nil {
		return tts.Audio{}, &tts.Error{
			Kind:     tts.KindUpstream,
			Provider: c.cfg.Provider,
			Status:   res.StatusCode,
			Message:  c.label() + " answered audio speak cannot play: " + err.Error(),
			Err:      err,
		}
	}
	return audio, nil
}

// newRequest builds the speech request for req.
func (c *Client) newRequest(ctx context.Context, req tts.Request) (*http.Request, error) {
	body, err := json.Marshal(speechRequest{
		Model:          c.cfg.Model,
		Input:          req.Text,
		Voice:          req.Voice,
		ResponseFormat: c.cfg.Format,
		Speed:          req.Speed,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal speech request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.BaseURL+"/v1/audio/speech",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("build speech request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	return httpReq, nil
}

// label names the endpoint in reasons: "TTS engine" for the local one, the
// provider's name otherwise.
func (c *Client) label() string {
	if c.cfg.Local {
		return "TTS engine"
	}
	return c.cfg.Provider
}

// unanswered classifies a request that got no answer. Running out of time is
// the provider being slow, not the network failing: a remote model can take
// tens of seconds for one part.
func (c *Client) unanswered(limit time.Duration, err error) *tts.Error {
	if tts.TimedOut(err) {
		return c.slow(limit, err)
	}
	return c.unreachable(err)
}

// slow classifies an endpoint that did not answer within limit. It counts as
// upstream, not network: the endpoint was reached.
func (c *Client) slow(limit time.Duration, err error) *tts.Error {
	where := ""
	if c.cfg.Local {
		where = " (check t-man logs speak-tts)"
	}
	return &tts.Error{
		Kind:     tts.KindUpstream,
		Provider: c.cfg.Provider,
		Message:  fmt.Sprintf("%s did not answer within %s%s", c.label(), limit, where),
		Err:      err,
	}
}

// unreachable classifies an endpoint that could not be reached.
func (c *Client) unreachable(err error) *tts.Error {
	hint := ""
	if c.cfg.Local {
		hint = " (is the speak-tts agent running? t-man status speak-tts)"
	}
	return &tts.Error{
		Kind:     tts.KindNetwork,
		Provider: c.cfg.Provider,
		Message:  fmt.Sprintf("%s not reachable at %s%s: %v", c.label(), c.cfg.BaseURL, hint, err),
		Err:      err,
	}
}

// failedAfterAnswer classifies a 200 that carried no audio, or a stream
// broken off mid-body (err set). The endpoint hit an error after the status
// line was out, so its cause is only in the endpoint's own log. It counts as
// upstream, not network: the endpoint is reachable.
func (c *Client) failedAfterAnswer(status int, err error) *tts.Error {
	what := "returned empty audio"
	if err != nil {
		what = "broke off the audio stream (" + err.Error() + ")"
	}
	where := ""
	if c.cfg.Local {
		where = ", so the cause is in its log (t-man logs speak-tts)"
	}
	return &tts.Error{
		Kind:     tts.KindUpstream,
		Provider: c.cfg.Provider,
		Status:   status,
		Message:  c.label() + " " + what + " after answering" + where,
		Err:      err,
	}
}

// Reachable reports whether the endpoint answers a quick GET / within 1.5s:
// a cheap ping for speak_status and doctor on the local engine. It proves a
// process is listening, not that it can synthesize; the recorded tts.Health
// answers that.
func (c *Client) Reachable(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/", nil)
	if err != nil {
		return fmt.Errorf("build reachability probe: %w", err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return c.unreachable(err)
	}
	return res.Body.Close()
}
