// Package ttsclient is a thin HTTP client for the local Kokoro TTS engine
// (mlx-audio) that `speak serve` also reverse-proxies. The MCP playback engine
// fetches per-sentence WAV audio from it and plays the bytes with afplay —
// server-side synthesis stays in the engine, this package only speaks HTTP.
package ttsclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	speak "github.com/mad01/thismoon/services/speak"
)

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

// speechRequest is the /v1/audio/speech payload. response_format is always
// "wav": the engine's default mp3 path shells out to ffmpeg, which may be
// missing and then fails as a 200 with an empty body rather than an error.
type speechRequest struct {
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

// Synthesize returns WAV bytes for one chunk of text in the given voice.
func (c *Client) Synthesize(text, voice string) ([]byte, error) {
	body, err := json.Marshal(speechRequest{Input: text, Voice: voice, ResponseFormat: "wav"})
	if err != nil {
		return nil, fmt.Errorf("marshal speech request: %w", err)
	}
	req, err := http.NewRequest(
		http.MethodPost,
		c.baseURL+"/v1/audio/speech",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		// The one transport chokepoint every playback path goes through;
		// Hint points the reader at the operating doc from here.
		return nil, agentdoc.Hint(fmt.Errorf(
			"TTS engine not reachable at %s — is the speak-tts agent running? (t-man status speak-tts): %w",
			c.baseURL,
			err,
		), speak.Facts())
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("TTS engine returned %d", res.StatusCode)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New(
			"TTS engine returned empty audio (check the engine's response_format/ffmpeg)",
		)
	}
	return data, nil
}

// Reachable reports whether the engine answers a quick GET / within 1.5s. It
// mirrors the web half's /enginez probe so tools can warn before a long hang.
func (c *Client) Reachable() bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	res, err := client.Get(c.baseURL + "/")
	if err != nil {
		return false
	}
	_ = res.Body.Close()
	return res.StatusCode < 500
}
