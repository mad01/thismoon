// Package tts holds what every speech backend shares: the classified error a
// failed synthesis returns, and the health state speak reports on every
// surface (the web page, speak doctor, the MCP tools). Classifying at the
// backend boundary is what lets each surface say why speech failed (a
// rejected key reads differently from a stopped engine) instead of a bare
// status code.
package tts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Kind classifies why a synthesis failed, so each surface can say what to do
// about it. Serialized as a string in /enginez and speech error bodies.
type Kind string

const (
	// KindAuth means the backend refused the credentials (401, 403).
	KindAuth Kind = "auth"
	// KindQuota means the backend rate-limited or ran out of quota (429).
	KindQuota Kind = "quota"
	// KindModel means the model or voice does not exist on the backend (404).
	KindModel Kind = "model"
	// KindNetwork means the request never reached the backend: connection
	// refused, DNS failure, a connect timeout.
	KindNetwork Kind = "network"
	// KindConfig means speak itself is missing what it needs to ask: no
	// backend or no credentials configured.
	KindConfig Kind = "config"
	// KindUpstream means the backend answered with an error nothing more
	// specific fits (a 5xx, another 4xx, an empty audio body), or took the
	// connection and did not answer within the request timeout.
	KindUpstream Kind = "upstream"
)

// maxDetailBytes bounds how much of an error body is quoted in a reason, so
// a backend that answers with an HTML error page cannot flood the UI.
const maxDetailBytes = 300

// Error is a classified synthesis failure. Message is the human-readable
// reason every surface shows; Status is the backend's HTTP status, 0 when the
// request never got an answer.
type Error struct {
	Kind     Kind
	Provider string
	Status   int
	Message  string
	Err      error
}

// Error implements error. It is Message, since that already names the
// provider and the cause.
func (e *Error) Error() string { return e.Message }

// Unwrap exposes the underlying transport error, if any.
func (e *Error) Unwrap() error { return e.Err }

// KindForStatus maps a backend's HTTP error status to a Kind.
func KindForStatus(status int) Kind {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return KindAuth
	case http.StatusNotFound:
		return KindModel
	case http.StatusTooManyRequests:
		return KindQuota
	default:
		return KindUpstream
	}
}

// FromResponse classifies a backend's error answer. body is the start of the
// response body; the backend's own message is pulled out of it when it
// carries one, so the reason reads "engine returned 500: model not found"
// rather than just the status.
func FromResponse(provider, backend string, status int, body []byte) *Error {
	msg := fmt.Sprintf("%s returned %d", backend, status)
	if detail := Detail(body); detail != "" {
		msg += ": " + detail
	}
	return &Error{
		Kind:     KindForStatus(status),
		Provider: provider,
		Status:   status,
		Message:  msg,
	}
}

// Detail extracts a backend's error message from a response body: the
// OpenAI-style {"error": {"message"}}, FastAPI's {"detail"} (what mlx-audio
// sends), a bare {"message"}, or else the text itself, trimmed and capped.
func Detail(body []byte) string {
	var shaped struct {
		Error   json.RawMessage `json:"error"`
		Detail  json.RawMessage `json:"detail"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(body, &shaped) == nil {
		var nested struct {
			Message string `json:"message"`
		}
		switch {
		case json.Unmarshal(shaped.Error, &nested) == nil && nested.Message != "":
			return clip(nested.Message)
		case len(shaped.Error) > 0 && jsonString(shaped.Error) != "":
			return clip(jsonString(shaped.Error))
		case len(shaped.Detail) > 0 && jsonString(shaped.Detail) != "":
			return clip(jsonString(shaped.Detail))
		case len(shaped.Detail) > 0:
			return clip(string(shaped.Detail))
		case shaped.Message != "":
			return clip(shaped.Message)
		}
	}
	return clip(string(body))
}

// jsonString decodes raw as a JSON string, returning "" when it is not one.
func jsonString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// clip collapses whitespace and caps s at maxDetailBytes on a rune boundary.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxDetailBytes {
		return s
	}
	cut := maxDetailBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
