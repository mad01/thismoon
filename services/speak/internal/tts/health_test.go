package tts

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestHealthStartsUnknown(t *testing.T) {
	h := NewHealth("local", "kokoro")
	got := h.Snapshot()
	if got.Status != StatusUnknown || got.Provider != "local" || got.Model != "kokoro" {
		t.Errorf("fresh state = %+v, want unknown for local/kokoro", got)
	}
	if h.CheckedWithin(time.Hour) {
		t.Error("CheckedWithin = true before anything was recorded")
	}
}

// TestHealthStatusFollowsTheFailures pins the degraded/down contract: a
// failure that can pass on its own degrades until downAfter in a row, a
// failure that needs a fix is down at once, and one success clears either.
func TestHealthStatusFollowsTheFailures(t *testing.T) {
	quota := &Error{Kind: KindQuota, Message: "rate limited"}
	auth := &Error{Kind: KindAuth, Message: "key rejected"}
	cases := []struct {
		name     string
		outcomes []error
		want     Status
		wantKind Kind
	}{
		{"success", []error{nil}, StatusOK, ""},
		{"one rate limit", []error{quota}, StatusDegraded, KindQuota},
		{"rate limited downAfter times", []error{quota, quota, quota}, StatusDown, KindQuota},
		{"bad key", []error{auth}, StatusDown, KindAuth},
		{"recovered", []error{auth, nil}, StatusOK, ""},
		{"success resets the run", []error{quota, quota, nil, quota}, StatusDegraded, KindQuota},
		{"unclassified error", []error{errors.New("boom")}, StatusDegraded, KindUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHealth("local", "")
			for _, err := range tc.outcomes {
				h.Record(err)
			}
			got := h.Snapshot()
			if got.Status != tc.want || got.Kind != tc.wantKind {
				t.Errorf("state = %s/%s, want %s/%s", got.Status, got.Kind, tc.want, tc.wantKind)
			}
		})
	}
}

// TestHealthReasonIsTheCleanMessage pins that a wrapped *Error reports its
// own message, not the wrapping (the doctor hint a CLI error carries).
func TestHealthReasonIsTheCleanMessage(t *testing.T) {
	h := NewHealth("local", "")
	h.Record(fmt.Errorf("%w (run 'speak doctor' to diagnose)",
		&Error{Kind: KindNetwork, Message: "engine not reachable"}))
	if got := h.Snapshot().Reason; got != "engine not reachable" {
		t.Errorf("reason = %q, want the unwrapped message", got)
	}
}

func TestHealthCheckedWithin(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	h := NewHealth("local", "")
	h.now = func() time.Time { return now }
	h.Record(nil)

	now = now.Add(30 * time.Second)
	if !h.CheckedWithin(time.Minute) {
		t.Error("CheckedWithin(1m) = false 30s after a success")
	}
	now = now.Add(time.Minute)
	if h.CheckedWithin(time.Minute) {
		t.Error("CheckedWithin(1m) = true 90s after a success")
	}
}

func TestStateSummary(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{State{Status: StatusUnknown}, "unknown (nothing synthesized yet)"},
		{State{Status: StatusOK}, "ok"},
		{
			State{Status: StatusDown, Kind: KindNetwork, Reason: "engine not reachable"},
			"down (network): engine not reachable",
		},
	}
	for _, tc := range cases {
		if got := tc.state.Summary(); got != tc.want {
			t.Errorf("Summary(%+v) = %q, want %q", tc.state, got, tc.want)
		}
	}
}
