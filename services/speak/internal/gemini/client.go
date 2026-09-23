// Package gemini is the client for speech generation on the Gemini
// Developer API: POST {base}/v1beta/models/{model}:generateContent asking
// for the audio response modality, with the key in the x-goog-api-key
// header. It serves the same tts.Request to tts.Audio contract as ttsclient
// and, like it, returns every failure as a *tts.Error.
//
// The API has no speed control: tts.Request.Speed is ignored.
package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	speak "github.com/mad01/thismoon/services/speak"
	"github.com/mad01/thismoon/services/speak/internal/tts"
)

// maxErrorBody bounds how much of an error response is read for its
// message.
const maxErrorBody = 4 << 10

// requestTimeout bounds one synthesis attempt, as ttsclient's remoteTimeout
// does: the speech models answer with the whole clip at once, in 15 to 22
// seconds for a part of up to about 700 characters with tails near 45, and
// now and then a request stalls at random, so one that runs out is retried
// rather than waited out.
const requestTimeout = 90 * time.Second

// retryDelay is the pause before the one retry. Google documents that the
// speech models now and then return text instead of audio, failing the
// request with a 500 at random, and says to retry. An answer that arrives
// without audio is the same fault getting past the server, and a stalled
// request another random fault, so both get the same retry.
const retryDelay = 500 * time.Millisecond

// errRedirectRefused stops the client before it can re-send the request, and
// its API key header, to a redirect target.
var errRedirectRefused = errors.New("gemini: HTTP redirect refused")

// Config locates the API and the model to speak with.
type Config struct {
	Provider  string // the config block's name, reported in health and errors
	BaseURL   string // API root, without /v1beta
	APIKey    string // sent in x-goog-api-key when set
	APIKeyEnv string // named in the reason when the key is refused
	Model     string // a speech model, e.g. gemini-3.1-flash-tts-preview
}

// Client synthesizes speech with one Gemini model.
type Client struct {
	cfg        Config
	http       *http.Client
	timeout    time.Duration
	retryDelay time.Duration
}

// New returns a Client for cfg.
func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimSuffix(strings.TrimRight(cfg.BaseURL, "/"), "/v1beta")
	cfg.Model = strings.TrimPrefix(cfg.Model, "models/")
	return &Client{
		cfg: cfg,
		http: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirectRefused
			},
		},
		timeout:    requestTimeout,
		retryDelay: retryDelay,
	}
}

// Synthesize returns playable audio for one chunk of text. A 5xx, an answer
// without audio, or a request that timed out is retried once, when the
// caller's context has room for another attempt.
func (c *Client) Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error) {
	body, err := json.Marshal(newGenerateRequest(req))
	if err != nil {
		return tts.Audio{}, fmt.Errorf("marshal generateContent request: %w", err)
	}
	audio, err := c.attempt(ctx, body)
	if !retryable(err) || !tts.WaitToRetry(ctx, c.timeout, c.retryDelay) {
		return audio, hinted(err)
	}
	audio, retryErr := c.attempt(ctx, body)
	return audio, hinted(tts.Retried(err, retryErr))
}

// retryable reports a failure worth one more try: a server error, an answer
// that carried no audio for no stated reason, or running out of time.
func retryable(err error) bool {
	te, ok := errors.AsType[*tts.Error](err)
	if !ok {
		return false
	}
	return tts.TimedOut(te) || (te.Kind == tts.KindUpstream &&
		(te.Status >= http.StatusInternalServerError || errors.Is(te, errNoAudio)))
}

// hinted points the reader at speak's operating doc when the API gave no
// answer at all, unreachable or out of time.
func hinted(err error) error {
	te, ok := errors.AsType[*tts.Error](err)
	if !ok || (te.Kind != tts.KindNetwork && !tts.TimedOut(te)) {
		return err
	}
	return agentdoc.Hint(err, speak.Facts())
}

// attempt is one generateContent call.
func (c *Client) attempt(ctx context.Context, body []byte) (tts.Audio, error) {
	ctx, cancel, limit := tts.WithTimeout(ctx, c.timeout)
	defer cancel()
	endpoint := c.cfg.BaseURL + "/v1beta/models/" + url.PathEscape(c.cfg.Model) + ":generateContent"
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint,
		bytes.NewReader(body),
	)
	if err != nil {
		return tts.Audio{}, fmt.Errorf("build generateContent request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		// The header, never the ?key= query parameter: URLs end up in logs
		// and error messages.
		httpReq.Header.Set("x-goog-api-key", c.cfg.APIKey)
	}

	res, err := c.http.Do(httpReq)
	if err != nil {
		return tts.Audio{}, c.unanswered(limit, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		return tts.Audio{}, c.refused(res.StatusCode, detail)
	}
	var answer generateResponse
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
		if tts.TimedOut(err) {
			return tts.Audio{}, c.slow(limit, err)
		}
		return tts.Audio{}, c.upstream(
			res.StatusCode,
			"answered unreadable JSON: "+err.Error(),
			err,
		)
	}
	return c.audio(res.StatusCode, answer)
}

// refused classifies an error answer. Google answers a bad key with 400
// INVALID_ARGUMENT, not 401, so the reason code in its error details
// decides that it is an auth failure.
func (c *Client) refused(status int, body []byte) *tts.Error {
	failure := tts.FromResponse(c.cfg.Provider, c.cfg.Provider, status, body)
	if status == http.StatusBadRequest && keyRejected(body) {
		failure.Kind = tts.KindAuth
	}
	if failure.Kind == tts.KindAuth && c.cfg.APIKeyEnv != "" {
		failure.Message += " (check " + c.cfg.APIKeyEnv + ")"
	}
	return failure
}

// keyRejected reports a Google error body saying the API key is invalid.
func keyRejected(body []byte) bool {
	var shaped errorBody
	if json.Unmarshal(body, &shaped) != nil {
		return false
	}
	for _, d := range shaped.Error.Details {
		if d.Reason == "API_KEY_INVALID" {
			return true
		}
	}
	return strings.Contains(shaped.Error.Message, "API key not valid")
}

// errNoAudio marks an answer without audio, which is retried like a 5xx.
var errNoAudio = errors.New("gemini: no audio in the answer")

// audio pulls the clip out of a successful answer. Text blocked by a safety
// filter is reported as such; any other answer without audio is errNoAudio.
func (c *Client) audio(status int, answer generateResponse) (tts.Audio, error) {
	if reason := answer.PromptFeedback.BlockReason; reason != "" {
		return tts.Audio{}, c.upstream(status, "refused the text ("+reason+")", nil)
	}
	finish := "none"
	for _, cand := range answer.Candidates {
		if cand.FinishReason != "" {
			finish = cand.FinishReason
		}
		for _, p := range cand.Content.Parts {
			if p.InlineData == nil || p.InlineData.Data == "" {
				continue
			}
			return c.decode(status, p.InlineData)
		}
	}
	return tts.Audio{}, c.upstream(
		status,
		"returned no audio (finish reason "+finish+")",
		errNoAudio,
	)
}

// decode turns inline audio into playable Audio. The speech models send
// 16-bit mono PCM labelled audio/L16 with its rate. Despite what the L16
// type means elsewhere the samples are little-endian (Google's own examples
// write them straight into a WAV file), so tts.Normalize wraps them as is.
func (c *Client) decode(status int, data *inlineData) (tts.Audio, error) {
	raw, err := base64.StdEncoding.DecodeString(data.Data)
	if err != nil {
		return tts.Audio{}, c.upstream(status, "answered undecodable audio: "+err.Error(), err)
	}
	audio, err := tts.Normalize(raw, data.MimeType)
	if err != nil {
		return tts.Audio{}, c.upstream(
			status,
			"answered audio speak cannot play: "+err.Error(),
			err,
		)
	}
	return audio, nil
}

func (c *Client) upstream(status int, what string, err error) *tts.Error {
	return &tts.Error{
		Kind:     tts.KindUpstream,
		Provider: c.cfg.Provider,
		Status:   status,
		Message:  c.cfg.Provider + " " + what,
		Err:      err,
	}
}

// unanswered classifies a request that got no answer. Running out of time is
// the model being slow, not the network failing.
func (c *Client) unanswered(limit time.Duration, err error) *tts.Error {
	if tts.TimedOut(err) {
		return c.slow(limit, err)
	}
	return c.unreachable(err)
}

// slow classifies an API that did not answer within limit. It counts as
// upstream, not network: the API was reached.
func (c *Client) slow(limit time.Duration, err error) *tts.Error {
	return &tts.Error{
		Kind:     tts.KindUpstream,
		Provider: c.cfg.Provider,
		Message:  fmt.Sprintf("%s did not answer within %s", c.cfg.Provider, limit),
		Err:      err,
	}
}

// unreachable classifies an API that could not be reached.
func (c *Client) unreachable(err error) *tts.Error {
	return &tts.Error{
		Kind:     tts.KindNetwork,
		Provider: c.cfg.Provider,
		Message:  fmt.Sprintf("%s not reachable at %s: %v", c.cfg.Provider, c.cfg.BaseURL, err),
		Err:      err,
	}
}
