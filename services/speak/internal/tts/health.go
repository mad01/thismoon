package tts

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// downAfter is how many failures in a row turn a transient kind (quota,
// upstream) from degraded into down: one 503 is a blip, three in a row are an
// outage.
const downAfter = 3

// Status is the coarse health of the active backend. Serialized as a string.
type Status string

const (
	// StatusUnknown means nothing has been synthesized or probed yet.
	StatusUnknown Status = "unknown"
	// StatusOK means the last synthesis succeeded.
	StatusOK Status = "ok"
	// StatusDegraded means the last synthesis failed in a way that may pass
	// on its own (rate limit, backend 5xx) and fewer than downAfter in a row
	// have failed.
	StatusDegraded Status = "degraded"
	// StatusDown means speech will not work until something changes: bad
	// credentials, missing model, unreachable backend, missing config, or
	// downAfter transient failures in a row.
	StatusDown Status = "down"
)

// State is a point-in-time view of the backend's health: the JSON body of
// GET /enginez and the source of every "why is speech failing" message.
type State struct {
	Status    Status    `json:"status"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model,omitempty"`
	Kind      Kind      `json:"kind,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// Summary renders the state as one line, e.g.
// "down (network): TTS engine not reachable at http://127.0.0.1:8765".
func (s State) Summary() string {
	switch {
	case s.Status == StatusUnknown:
		return "unknown (nothing synthesized yet)"
	case s.Reason == "":
		return string(s.Status)
	default:
		return fmt.Sprintf("%s (%s): %s", s.Status, s.Kind, s.Reason)
	}
}

// Health tracks the active backend's State from the outcome of every
// synthesis, real or probe. It is safe for concurrent use.
type Health struct {
	now func() time.Time

	mu       sync.Mutex
	state    State
	failures int // consecutive failed syntheses
}

// NewHealth returns a tracker for the named provider and model, starting at
// StatusUnknown.
func NewHealth(provider, model string) *Health {
	return &Health{
		now:   time.Now,
		state: State{Status: StatusUnknown, Provider: provider, Model: model},
	}
}

// Record folds one synthesis outcome into the state: nil is a success, an
// error is classified by its *Error kind (unclassified errors count as
// upstream).
func (h *Health) Record(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.CheckedAt = h.now()
	if err == nil {
		h.failures = 0
		h.state.Status, h.state.Kind, h.state.Reason = StatusOK, "", ""
		return
	}
	h.failures++
	kind, reason := KindUpstream, err.Error()
	if te, ok := errors.AsType[*Error](err); ok {
		kind, reason = te.Kind, te.Message
	}
	h.state.Status, h.state.Kind, h.state.Reason = statusFor(kind, h.failures), kind, reason
}

// Snapshot returns the current state.
func (h *Health) Snapshot() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// CheckedWithin reports whether a synthesis outcome was recorded less than
// maxAge ago, i.e. whether the state is fresh enough to answer from without
// probing.
func (h *Health) CheckedWithin(maxAge time.Duration) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return !h.state.CheckedAt.IsZero() && h.now().Sub(h.state.CheckedAt) < maxAge
}

// statusFor maps a failure kind and the consecutive-failure count to a
// Status: transient kinds degrade until downAfter failures in a row, the
// rest are down at once.
func statusFor(kind Kind, failures int) Status {
	switch kind {
	case KindQuota, KindUpstream:
		if failures < downAfter {
			return StatusDegraded
		}
	}
	return StatusDown
}
