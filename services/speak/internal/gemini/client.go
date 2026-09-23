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

// requestTimeout bounds one synthesis attempt, as in ttsclient.
const requestTimeout = 30 * time.Second

// retryDelay is the pause before the one retry. Google documents that the
// speech models now and then return text instead of audio, failing the
// request with a 500 at random, and says to retry. An answer that arrives
// without audio is the same fault getting past the server, so it gets the
// same retry.
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
	retryDelay time.Duration
}

// New returns a Client for cfg.
func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimSuffix(strings.TrimRight(cfg.BaseURL, "/"), "/v1beta")
	cfg.Model = strings.TrimPrefix(cfg.Model, "models/")
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirectRefused
			},
		},
		retryDelay: retryDelay,
	}
}

// Synthesize returns playable audio for one chunk of text. A 5xx, or an
// answer without audio, is retried once.
func (c *Client) Synthesize(ctx context.Context, req tts.Request) (tts.Audio, error) {
	body, err := json.Marshal(newGenerateRequest(req))
	if err != nil {
		return tts.Audio{}, fmt.Errorf("marshal generateContent request: %w", err)
	}
	audio, err := c.attempt(ctx, body)
	if te, ok := errors.AsType[*tts.Error](err); !ok || !retryable(te) {
		return audio, err
	}
	select {
	case <-ctx.Done():
		return tts.Audio{}, err
	case <-time.After(c.retryDelay):
	}
	return c.attempt(ctx, body)
}

// retryable reports a failure worth one more try: a server error, or an
// answer that carried no audio for no stated reason.
func retryable(err *tts.Error) bool {
	return err.Kind == tts.KindUpstream && (err.Status >= http.StatusInternalServerError ||
		errors.Is(err, errNoAudio))
}

// attempt is one generateContent call.
func (c *Client) attempt(ctx context.Context, body []byte) (tts.Audio, error) {
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
		return tts.Audio{}, agentdoc.Hint(c.unreachable(err), speak.Facts())
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= http.StatusBadRequest {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBody))
		return tts.Audio{}, c.refused(res.StatusCode, detail)
	}
	var answer generateResponse
	if err := json.NewDecoder(res.Body).Decode(&answer); err != nil {
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

// unreachable classifies an API that never answered.
func (c *Client) unreachable(err error) *tts.Error {
	return &tts.Error{
		Kind:     tts.KindNetwork,
		Provider: c.cfg.Provider,
		Message:  fmt.Sprintf("%s not reachable at %s: %v", c.cfg.Provider, c.cfg.BaseURL, err),
		Err:      err,
	}
}
